package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	netinternal "github.com/zsrv/supermicro-product-key/pkg/net"
	"github.com/zsrv/supermicro-product-key/pkg/nonjson"
	"github.com/zsrv/supermicro-product-key/pkg/oob"
)

type GenerateRequest struct {
	MAC string `json:"mac"`
}

type KeyResult struct {
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

type GenerateResponse struct {
	MACRaw       string      `json:"mac_raw"`
	MACClean     string      `json:"mac_clean"`
	MACFormatted string      `json:"mac_formatted"`
	Keys         []KeyResult `json:"keys"`
	Error        string      `json:"error,omitempty"`
}

type DecodeRequest struct {
	MAC string `json:"mac"`
	Key string `json:"key"`
}

type DecodeResponse struct {
	FormatVersion      byte        `json:"format_version"`
	SoftwareIdentifier string      `json:"software_identifier"`
	SKU                string      `json:"sku"`
	SoftwareVersion    string      `json:"software_version"`
	InvoiceNumber      string      `json:"invoice_number"`
	CreationDate       string      `json:"creation_date"`
	ExpirationDate     string      `json:"expiration_date"`
	SecretData         string      `json:"secret_data"`
	Checksum           byte        `json:"checksum"`
	Error              string      `json:"error,omitempty"`
}

type BruteForceRequest struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

type BruteForceResponse struct {
	MAC   string `json:"mac,omitempty"`
	Error string `json:"error,omitempty"`
}

type ActivateRequest struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
	Password string `json:"password"`
	Key      string `json:"key"`
}

type ActivateResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

type AutoActivateRequest struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
	Password string `json:"password"`
	SKU      string `json:"sku"` // e.g. "SFT-DCMS-SINGLE", "SFT-OOB-LIC", or "ALL"
}

type AutoActivateResponse struct {
	Success      bool        `json:"success"`
	MACDetected  string      `json:"mac_detected"`
	MACClean     string      `json:"mac_clean"`
	MACFormatted string      `json:"mac_formatted"`
	SKU          string      `json:"sku"`
	GeneratedKey string      `json:"generated_key"`
	AllKeys      []KeyResult `json:"all_keys,omitempty"`
	Output       string      `json:"output"`
	Error        string      `json:"error,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.HandleFunc("/api/generate", handleGenerate)
	http.HandleFunc("/api/decode", handleDecode)
	http.HandleFunc("/api/bruteforce", handleBruteForce)
	http.HandleFunc("/api/skus", handleListSKUs)
	http.HandleFunc("/api/activate", handleActivate)
	http.HandleFunc("/api/auto-activate", handleAutoActivate)

	log.Printf("Server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var macStr string
	if r.Method == http.MethodPost {
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			macStr = req.MAC
		}
	} else {
		macStr = r.URL.Query().Get("mac")
	}

	if macStr == "" {
		json.NewEncoder(w).Encode(GenerateResponse{Error: "MAC-адрес не указан"})
		return
	}

	// Clean input MAC address (remove colons, dashes, spaces)
	cleanMACInput := cleanMACString(macStr)

	hwAddr, err := netinternal.ParseHardwareAddr(cleanMACInput)
	if err != nil {
		json.NewEncoder(w).Encode(GenerateResponse{
			Error:  fmt.Sprintf("Некорректный MAC-адрес '%s'. Должно быть 12 HEX-символов.", macStr),
			MACRaw: macStr,
		})
		return
	}

	macClean := hwAddr.String()
	macFormatted := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		hwAddr[0], hwAddr[1], hwAddr[2], hwAddr[3], hwAddr[4], hwAddr[5])

	var results []KeyResult

	// 1. OOB Key (SFT-OOB-LIC)
	oobKey, err := oob.EncodeOOBProductKey(hwAddr)
	if err == nil {
		results = append(results, KeyResult{
			SKU:         "SFT-OOB-LIC",
			Name:        "OOB Management License (Web IPMI 24-char)",
			Type:        "oob",
			Key:         oobKey.String(),
			Description: "Разблокирует BIOS update через IPMI Web, RAID и HTML5 KVM.",
		})
	}

	// 2. Non-JSON Keys
	skuDescriptions := map[string]string{
		"SFT-DCMS-SINGLE":    "Полный пакет Data Center Management Suite (SUM + SPM + SCM)",
		"SFT-SUM-LIC":        "Supermicro Update Manager (SUM) License",
		"SFT-SPM-LIC":        "Supermicro Power Manager (SPM) License",
		"SFT-SCM-LIC":        "Supermicro Cloud Manager (SCM) License",
		"SFT-DCMS-SITE":      "Data Center Management Suite - Site License",
		"SFT-DCMS-CALL-HOME": "DCMS Call-Home Service License",
		"SFT-DCMS-SVC-KEY":   "DCMS Service Key License",
		"SFT-SDDC-SINGLE":    "Software Defined Data Center Single License",
	}

	for _, sid := range nonjson.SoftwareIdentifiers.List() {
		if sid.SKU == "" {
			continue
		}

		pk := nonjson.NewDefaultProductKey()
		pk.SoftwareIdentifier = *sid

		encodedKey, err := pk.Encode(hwAddr)
		if err != nil {
			continue
		}

		desc := skuDescriptions[sid.SKU]
		if desc == "" {
			desc = fmt.Sprintf("Non-JSON ключ для SKU %s (ID %d)", sid.SKU, sid.ID)
		}

		results = append(results, KeyResult{
			SKU:         sid.SKU,
			Name:        sid.DisplayName,
			Type:        "nonjson",
			Key:         encodedKey,
			Description: desc,
		})
	}

	json.NewEncoder(w).Encode(GenerateResponse{
		MACRaw:       macStr,
		MACClean:     macClean,
		MACFormatted: macFormatted,
		Keys:         results,
	})
}

func handleAutoActivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AutoActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(AutoActivateResponse{Success: false, Error: "Некорректный JSON запрос"})
		return
	}

	if req.IP == "" {
		json.NewEncoder(w).Encode(AutoActivateResponse{Success: false, Error: "IP-адрес BMC обязателен"})
		return
	}

	if req.Username == "" {
		req.Username = "ADMIN"
	}
	if req.Password == "" {
		req.Password = "ADMIN"
	}
	if req.SKU == "" {
		req.SKU = "SFT-DCMS-SINGLE"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Step 1: Automatically fetch/read BMC MAC address from BMC
	macStr, logStep1, err := fetchBMCMAC(ctx, req.IP, req.Username, req.Password)
	if err != nil {
		json.NewEncoder(w).Encode(AutoActivateResponse{
			Success: false,
			Error:   fmt.Sprintf("Не удалось автоматически считать BMC MAC адрес с %s: %v", req.IP, err),
			Output:  logStep1,
		})
		return
	}

	cleanMAC := cleanMACString(macStr)
	hwAddr, err := netinternal.ParseHardwareAddr(cleanMAC)
	if err != nil {
		json.NewEncoder(w).Encode(AutoActivateResponse{
			Success: false,
			Error:   fmt.Sprintf("Распознанный MAC-адрес '%s' невалиден: %v", macStr, err),
			Output:  logStep1,
		})
		return
	}

	macClean := hwAddr.String()
	macFormatted := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		hwAddr[0], hwAddr[1], hwAddr[2], hwAddr[3], hwAddr[4], hwAddr[5])

	// Step 2: Generate Product Key
	var generatedKey string
	var allKeys []KeyResult

	if req.SKU == "SFT-OOB-LIC" {
		oobKey, err := oob.EncodeOOBProductKey(hwAddr)
		if err != nil {
			json.NewEncoder(w).Encode(AutoActivateResponse{
				Success: false,
				Error:   fmt.Sprintf("Ошибка генерации OOB ключа: %v", err),
			})
			return
		}
		generatedKey = oobKey.String()
	} else {
		// Non-JSON SKU
		sid, err := nonjson.SoftwareIdentifiers.BySKU(req.SKU)
		if err != nil {
			// Fallback to SFT-DCMS-SINGLE
			sid = nonjson.SoftwareIdentifiers.ALL
		}
		pk := nonjson.NewDefaultProductKey()
		pk.SoftwareIdentifier = *sid
		generatedKey, err = pk.Encode(hwAddr)
		if err != nil {
			json.NewEncoder(w).Encode(AutoActivateResponse{
				Success: false,
				Error:   fmt.Sprintf("Ошибка генерации ключа %s: %v", req.SKU, err),
			})
			return
		}
	}

	// Generate all keys to return in response as well
	oobKey, _ := oob.EncodeOOBProductKey(hwAddr)
	if oobKey != nil {
		allKeys = append(allKeys, KeyResult{
			SKU:         "SFT-OOB-LIC",
			Name:        "OOB Management License",
			Type:        "oob",
			Key:         oobKey.String(),
			Description: "Unlocks BIOS update via IPMI Web",
		})
	}
	for _, sid := range nonjson.SoftwareIdentifiers.List() {
		if sid.SKU == "" {
			continue
		}
		pk := nonjson.NewDefaultProductKey()
		pk.SoftwareIdentifier = *sid
		kStr, err := pk.Encode(hwAddr)
		if err == nil {
			allKeys = append(allKeys, KeyResult{
				SKU:         sid.SKU,
				Name:        sid.DisplayName,
				Type:        "nonjson",
				Key:         kStr,
				Description: sid.DisplayName,
			})
		}
	}

	// Step 3: Run SUM Activation
	sumPath := findSUMBinary()
	if sumPath == "" {
		json.NewEncoder(w).Encode(AutoActivateResponse{
			Success:      false,
			MACDetected:  macStr,
			MACClean:     macClean,
			MACFormatted: macFormatted,
			SKU:          req.SKU,
			GeneratedKey: generatedKey,
			AllKeys:      allKeys,
			Error:        "Утилита SUM не найдена на сервере (/app/sum). Ключ сгенерирован, но не активирован.",
			Output:       logStep1,
		})
		return
	}

	cmd := exec.CommandContext(ctx, sumPath, "-i", req.IP, "-u", req.Username, "-p", req.Password, "-c", "ActivateProductKey", "--key", generatedKey)
	cmd.Dir = filepath.Dir(sumPath)

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	fullOutput := fmt.Sprintf("[ШАГ 1] Автоматически считан BMC MAC: %s (%s)\n[ШАГ 2] Сгенерирован ключ (%s): %s\n[ШАГ 3] Результат выполнения SUM:\n%s",
		macFormatted, macClean, req.SKU, generatedKey, outputStr)

	if err != nil {
		json.NewEncoder(w).Encode(AutoActivateResponse{
			Success:      false,
			MACDetected:  macStr,
			MACClean:     macClean,
			MACFormatted: macFormatted,
			SKU:          req.SKU,
			GeneratedKey: generatedKey,
			AllKeys:      allKeys,
			Output:       fullOutput,
			Error:        fmt.Sprintf("Ошибка при выполнении SUM ActivateProductKey: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(AutoActivateResponse{
		Success:      true,
		MACDetected:  macStr,
		MACClean:     macClean,
		MACFormatted: macFormatted,
		SKU:          req.SKU,
		GeneratedKey: generatedKey,
		AllKeys:      allKeys,
		Output:       fullOutput,
	})
}

func fetchBMCMAC(ctx context.Context, ip, username, password string) (string, string, error) {
	var logs []string

	// Strategy 1: Use SUM tool `-c GetBmcInfo`
	sumPath := findSUMBinary()
	if sumPath != "" {
		logs = append(logs, "[SUM] Запрос GetBmcInfo...")
		cmd := exec.CommandContext(ctx, sumPath, "-i", ip, "-u", username, "-p", password, "-c", "GetBmcInfo")
		cmd.Dir = filepath.Dir(sumPath)
		out, err := cmd.CombinedOutput()
		outStr := string(out)
		logs = append(logs, outStr)

		if err == nil || len(outStr) > 0 {
			mac := extractMACFromText(outStr)
			if mac != "" {
				return mac, strings.Join(logs, "\n"), nil
			}
		}
	}

	// Strategy 2: Redfish API
	logs = append(logs, "[Redfish] Подключение к https://" + ip + "/redfish/v1/Managers/1...")
	mac, rLogs, err := fetchMACViaRedfish(ctx, ip, username, password)
	logs = append(logs, rLogs)
	if err == nil && mac != "" {
		return mac, strings.Join(logs, "\n"), nil
	}

	return "", strings.Join(logs, "\n"), fmt.Errorf("не удалось считать MAC ни через SUM, ни через Redfish")
}

func fetchMACViaRedfish(ctx context.Context, ip, username, password string) (string, string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	urls := []string{
		fmt.Sprintf("https://%s/redfish/v1/Managers/1", ip),
		fmt.Sprintf("https://%s/redfish/v1/Managers/1/EthernetInterfaces/1", ip),
		fmt.Sprintf("https://%s/redfish/v1/Systems/1", ip),
	}

	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			continue
		}
		req.SetBasicAuth(username, password)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		mac := extractMACFromText(string(body))
		if mac != "" {
			return mac, fmt.Sprintf("MAC найден в %s: %s", u, mac), nil
		}
	}

	return "", "Redfish не вернул MAC-адрес", fmt.Errorf("mac not found in redfish")
}

func extractMACFromText(text string) string {
	// 1. Look for explicit BMC MAC patterns first
	reExplicit := regexp.MustCompile(`(?i)(?:BMC\s*MAC|MAC\s*Address|PermanentMACAddress)\s*["':=]?\s*([0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}|[0-9A-Fa-f]{12})`)
	matches := reExplicit.FindStringSubmatch(text)
	if len(matches) > 1 {
		return cleanMACString(matches[1])
	}

	// 2. Generic MAC pattern regex
	reGeneric := regexp.MustCompile(`(?i)\b([0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2})\b`)
	genMatches := reGeneric.FindStringSubmatch(text)
	if len(genMatches) > 1 {
		return cleanMACString(genMatches[1])
	}

	// 3. Raw 12-char hex string
	reHex12 := regexp.MustCompile(`(?i)\b([0-9A-Fa-f]{12})\b`)
	hexMatches := reHex12.FindAllString(text, -1)
	for _, m := range hexMatches {
		clean := cleanMACString(m)
		if len(clean) == 12 {
			return clean
		}
	}

	return ""
}

func cleanMACString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ".", "")
	return strings.ToUpper(s)
}

func handleDecode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DecodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(DecodeResponse{Error: "Invalid JSON request body"})
		return
	}

	cleanMAC := cleanMACString(req.MAC)
	hwAddr, err := netinternal.ParseHardwareAddr(cleanMAC)
	if err != nil {
		json.NewEncoder(w).Encode(DecodeResponse{Error: fmt.Sprintf("Invalid MAC address: %v", err)})
		return
	}

	pk, err := nonjson.ParseEncodedProductKey(req.Key, hwAddr)
	if err != nil {
		json.NewEncoder(w).Encode(DecodeResponse{Error: fmt.Sprintf("Failed to decode key: %v", err)})
		return
	}

	json.NewEncoder(w).Encode(DecodeResponse{
		FormatVersion:      pk.FormatVersion,
		SoftwareIdentifier: pk.SoftwareIdentifier.DisplayName,
		SKU:                pk.SoftwareIdentifier.SKU,
		SoftwareVersion:    pk.SoftwareVersion,
		InvoiceNumber:      pk.InvoiceNumber,
		CreationDate:       pk.CreationDate.Format("2006-01-02T15:04:05Z"),
		ExpirationDate:     pk.ExpirationDate.Format("2006-01-02T15:04:05Z"),
		SecretData:         fmt.Sprintf("%x", pk.SecretData),
		Checksum:           pk.Checksum,
	})
}

func handleBruteForce(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BruteForceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(BruteForceResponse{Error: "Invalid JSON request body"})
		return
	}

	if req.Key == "" {
		json.NewEncoder(w).Encode(BruteForceResponse{Error: "Key is required"})
		return
	}

	var foundMAC string
	var err error

	if req.Type == "nonjson" {
		foundMAC, err = nonjson.BruteForceMACAddress(req.Key)
	} else {
		foundMAC, err = oob.BruteForceMACAddress(req.Key)
	}

	if err != nil {
		json.NewEncoder(w).Encode(BruteForceResponse{Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(BruteForceResponse{MAC: foundMAC})
}

func handleListSKUs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	list := nonjson.SoftwareIdentifiers.List()
	json.NewEncoder(w).Encode(list)
}

func handleActivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(ActivateResponse{Success: false, Error: "Invalid JSON request body"})
		return
	}

	if req.IP == "" || req.Key == "" {
		json.NewEncoder(w).Encode(ActivateResponse{Success: false, Error: "BMC IP and Product Key are required"})
		return
	}

	if req.Username == "" {
		req.Username = "ADMIN"
	}
	if req.Password == "" {
		req.Password = "ADMIN"
	}

	sumPath := findSUMBinary()
	if sumPath == "" {
		json.NewEncoder(w).Encode(ActivateResponse{
			Success: false,
			Error:   "SUM tool binary not found on server.",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, sumPath, "-i", req.IP, "-u", req.Username, "-p", req.Password, "-c", "ActivateProductKey", "--key", req.Key)
	cmd.Dir = filepath.Dir(sumPath)

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if err != nil {
		json.NewEncoder(w).Encode(ActivateResponse{
			Success: false,
			Output:  outputStr,
			Error:   fmt.Sprintf("SUM execution error: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(ActivateResponse{
		Success: true,
		Output:  outputStr,
	})
}

func findSUMBinary() string {
	if envPath := os.Getenv("SUM_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	candidates := []string{
		"/app/sum",
		"/app/sum_tool/sum",
		"./sum",
		"./sum_tool/sum",
		"sum_2.15.0_Linux_x86_64_20251104/sum_2.15.0_Linux_x86_64/sum",
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}

	return ""
}
