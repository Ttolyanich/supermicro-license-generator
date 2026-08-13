package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	netinternal "github.com/zsrv/supermicro-product-key/pkg/net"
	"github.com/zsrv/supermicro-product-key/pkg/nonjson"
	"github.com/zsrv/supermicro-product-key/pkg/oob"
)

//go:embed static/*
var staticFS embed.FS

// bruteForceSem limits MAC bruteforce searches to one at a time. Each search is
// CPU-bound (up to ~117M AES decryptions), so serializing prevents concurrent
// requests from exhausting the host.
var bruteForceSem = make(chan struct{}, 1)

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
	MAC      string `json:"mac"`
	SKU      string `json:"sku"`
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

	// Host defaults to localhost so the standalone .exe is not reachable from
	// the network. The Docker image sets HOST=0.0.0.0 explicitly to publish
	// the port. This mitigates the SSRF surface of the activation endpoints.
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	// Serve embedded static files
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to initialize embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFS)))
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/decode", handleDecode)
	mux.HandleFunc("/api/bruteforce", handleBruteForce)
	mux.HandleFunc("/api/skus", handleListSKUs)
	mux.HandleFunc("/api/activate", handleActivate)
	mux.HandleFunc("/api/auto-activate", handleAutoActivate)
	mux.HandleFunc("/api/sum/status", handleSUMStatus)
	mux.HandleFunc("/api/sum/upload", handleSUMUpload)

	handler := withSecurity(mux)

	addr := host + ":" + port
	log.Printf("==========================================================")
	log.Printf("Supermicro License Generator & SUM Activator Web App")
	log.Printf("Running on OS: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("Listening on: %s", addr)
	log.Printf("Web Interface URL: http://localhost:%s", port)
	if authUser, _ := basicAuthCreds(); authUser != "" {
		log.Printf("HTTP Basic Auth: ENABLED (user %q)", authUser)
	}
	sumPath := findSUMBinary()
	if sumPath != "" {
		log.Printf("SUM Binary Tool Detected: %s", sumPath)
	} else {
		log.Printf("WARNING: SUM Tool not found. Key generation works, but direct SUM activation requires SUM binary.")
	}
	log.Printf("==========================================================")

	// Automatically open the default browser in desktop/GUI environments.
	// Skipped on headless/server deployments (set NO_BROWSER=1, as the
	// Docker image does) to avoid a spurious "xdg-open not found" error.
	if os.Getenv("NO_BROWSER") == "" {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openBrowser(fmt.Sprintf("http://localhost:%s", port))
		}()
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No overall WriteTimeout: activation and bruteforce handlers may run
		// for up to ~2 minutes and stream their result at the end.
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// basicAuthCreds returns the optional HTTP Basic Auth credentials configured
// via BASIC_AUTH_USER / BASIC_AUTH_PASS. When the user is empty, auth is off.
func basicAuthCreds() (string, string) {
	return os.Getenv("BASIC_AUTH_USER"), os.Getenv("BASIC_AUTH_PASS")
}

// withSecurity wraps the mux with cross-cutting protections:
//   - a request body size limit,
//   - optional HTTP Basic Auth,
//   - a CSRF guard for state-changing API calls (JSON content-type + a
//     same-origin check on Sec-Fetch-Site / Origin), which stops a malicious
//     web page from driving the activation endpoints of a locally bound server.
func withSecurity(next http.Handler) http.Handler {
	authUser, authPass := basicAuthCreds()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The SUM archive upload is a large multipart request; it sets its own
		// (much higher) body limit and is not JSON. Everything else is capped
		// here to keep JSON decoding cheap and bounded.
		isUpload := r.URL.Path == "/api/sum/upload"
		if !isUpload {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
		}

		if authUser != "" {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(authUser)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(authPass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Supermicro License Generator"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method == http.MethodPost {
			// Same-origin is enforced for every state-changing API call,
			// including the upload (which installs an executable).
			if !isSameOrigin(r) {
				http.Error(w, "Cross-origin request blocked", http.StatusForbidden)
				return
			}
			if !isUpload {
				if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
					http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isSameOrigin rejects requests a browser marks as cross-site. It trusts the
// Fetch metadata header when present (modern browsers), and otherwise falls
// back to comparing the Origin host with the request Host. Non-browser
// clients (curl, SUM tooling) send neither header and are allowed through.
func isSameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "same-site", "none":
		return true
	case "cross-site":
		return false
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // no Origin header: not a browser-initiated cross-site call
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

func openBrowser(urlStr string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("cmd", "/c", "start", urlStr).Start()
	case "darwin":
		err = exec.Command("open", urlStr).Start()
	default:
		err = exec.Command("xdg-open", urlStr).Start()
	}
	if err != nil {
		log.Printf("Could not open browser automatically: %v", err)
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

	cleanMACInput := cleanMACString(macStr)

	hwAddr, err := netinternal.ParseMAC(cleanMACInput)
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

	var macStr string
	var logStep1 string

	if strings.TrimSpace(req.MAC) != "" {
		macStr = req.MAC
		logStep1 = fmt.Sprintf("[Ввод пользователя] Использован переданный BMC MAC адрес: %s", macStr)
	} else {
		var err error
		macStr, logStep1, err = fetchBMCMAC(ctx, req.IP, req.Username, req.Password)
		if err != nil {
			json.NewEncoder(w).Encode(AutoActivateResponse{
				Success: false,
				Error:   fmt.Sprintf("Не удалось автоматически считать BMC MAC адрес с %s: %v. Вы можете указать MAC вручную.", req.IP, err),
				Output:  logStep1,
			})
			return
		}
	}

	cleanMAC := cleanMACString(macStr)
	hwAddr, err := netinternal.ParseMAC(cleanMAC)
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
		sid, err := nonjson.SoftwareIdentifiers.BySKU(req.SKU)
		if err != nil {
			// Fail loudly instead of silently activating a different license
			// than the caller asked for.
			json.NewEncoder(w).Encode(AutoActivateResponse{
				Success:      false,
				MACDetected:  macStr,
				MACClean:     macClean,
				MACFormatted: macFormatted,
				SKU:          req.SKU,
				Error:        fmt.Sprintf("Неизвестный SKU '%s'. Выберите один из поддерживаемых типов лицензий.", req.SKU),
				Output:       logStep1,
			})
			return
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
			Error:        "Утилита SUM не найдена. Ключ сгенерирован, но не активирован.",
			Output:       logStep1,
		})
		return
	}

	cmd := exec.CommandContext(ctx, sumPath, "-i", req.IP, "-u", req.Username, "-p", req.Password, "-c", "ActivateProductKey", "--key", generatedKey)
	cmd.Dir = filepath.Dir(sumPath)

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	fullOutput := fmt.Sprintf("[ШАГ 1] BMC MAC адрес: %s (%s)\n[ШАГ 2] Сгенерирован ключ (%s): %s\n[ШАГ 3] Результат выполнения SUM:\n%s",
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

	// Strategy 1: SUM tool `-c GetBmcInfo`
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

	// Strategy 2: IPMI Web CGI scraping (/cgi/login.cgi -> /cgi/url_flag.cgi?url_flag=sys_info)
	logs = append(logs, "[IPMI Web CGI] Авторизация на https://"+ip+"/cgi/login.cgi...")
	macCGI, logCGI, errCGI := fetchMACViaCGILogin(ctx, ip, username, password)
	logs = append(logs, logCGI)
	if errCGI == nil && macCGI != "" {
		return macCGI, strings.Join(logs, "\n"), nil
	}

	// Strategy 3: Redfish API
	logs = append(logs, "[Redfish] Подключение к https://"+ip+"/redfish/v1/Managers/1...")
	mac, rLogs, err := fetchMACViaRedfish(ctx, ip, username, password)
	logs = append(logs, rLogs)
	if err == nil && mac != "" {
		return mac, strings.Join(logs, "\n"), nil
	}

	return "", strings.Join(logs, "\n"), fmt.Errorf("не удалось считать MAC ни через SUM, ни через IPMI Web CGI, ни через Redfish")
}

func fetchMACViaCGILogin(ctx context.Context, ip, username, password string) (string, string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   12 * time.Second,
	}

	loginURL := fmt.Sprintf("https://%s/cgi/login.cgi", ip)
	formData := url.Values{}
	formData.Set("name", username)
	formData.Set("pwd", password)

	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("CGI login failed: %v", err)
	}
	defer resp.Body.Close()

	sysInfoURL := fmt.Sprintf("https://%s/cgi/url_flag.cgi?url_flag=sys_info", ip)
	reqInfo, err := http.NewRequestWithContext(ctx, "GET", sysInfoURL, nil)
	if err != nil {
		return "", "", err
	}

	respInfo, err := client.Do(reqInfo)
	if err != nil {
		return "", "", fmt.Errorf("CGI sys_info failed: %v", err)
	}
	bodyInfo, _ := io.ReadAll(respInfo.Body)
	respInfo.Body.Close()

	mac := extractMACFromText(string(bodyInfo))
	if mac != "" {
		return mac, fmt.Sprintf("[IPMI Web CGI] MAC успешно найден на странице sys_info: %s", mac), nil
	}

	return "", "", fmt.Errorf("MAC не найден в ответе sys_info")
}

func fetchMACViaRedfish(ctx context.Context, ip, username, password string) (string, string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	// EthernetInterfaces is queried first: it is the resource that actually
	// carries the BMC MAC in a labelled MACAddress/PermanentMACAddress field,
	// so a match there is trustworthy. The Managers/Systems endpoints are
	// fallbacks only.
	urls := []string{
		fmt.Sprintf("https://%s/redfish/v1/Managers/1/EthernetInterfaces/1", ip),
		fmt.Sprintf("https://%s/redfish/v1/Managers/1", ip),
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
		status := resp.StatusCode
		resp.Body.Close()

		// Only parse successful responses. A 401/404 body must not be scanned
		// for hex that could be mistaken for a MAC address.
		if status < 200 || status >= 300 {
			continue
		}

		mac := extractMACFromText(string(body))
		if mac != "" {
			return mac, fmt.Sprintf("MAC найден в %s: %s", u, mac), nil
		}
	}

	return "", "Redfish не вернул MAC-адрес", fmt.Errorf("mac not found in redfish")
}

// extractMACFromText pulls a BMC MAC address out of SUM output, IPMI Web CGI
// pages, or Redfish JSON. It deliberately only accepts MACs that are either
// explicitly labelled (BMC MAC / MAC Address / PermanentMACAddress) or written
// in canonical colon/dash-separated form. The previous bare "any 12 hex
// characters" fallback was removed: it happily matched a slice of a UUID,
// serial number, or signature and could yield a key for the wrong MAC.
func extractMACFromText(text string) string {
	reExplicit := regexp.MustCompile(`(?i)(?:BMC\s*MAC|MAC\s*Address|PermanentMACAddress)\s*["':=]?\s*([0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}|[0-9A-Fa-f]{12})`)
	matches := reExplicit.FindStringSubmatch(text)
	if len(matches) > 1 {
		return cleanMACString(matches[1])
	}

	reGeneric := regexp.MustCompile(`(?i)\b([0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2}[:-][0-9A-Fa-f]{2})\b`)
	genMatches := reGeneric.FindStringSubmatch(text)
	if len(genMatches) > 1 {
		return cleanMACString(genMatches[1])
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
	hwAddr, err := netinternal.ParseMAC(cleanMAC)
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

	// A full search is up to 7 x 16.7M decryptions and is CPU-bound. Limit it
	// to one at a time so concurrent requests cannot pile up and exhaust the
	// CPU, and bound each search with a timeout tied to the request context so
	// a client disconnect stops the worker goroutines.
	select {
	case bruteForceSem <- struct{}{}:
		defer func() { <-bruteForceSem }()
	default:
		json.NewEncoder(w).Encode(BruteForceResponse{Error: "Сервер уже выполняет подбор MAC. Повторите попытку позже."})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var foundMAC string
	var err error

	if req.Type == "nonjson" {
		foundMAC, err = nonjson.BruteForceMACAddressContext(ctx, req.Key)
	} else {
		foundMAC, err = oob.BruteForceMACAddressContext(ctx, req.Key)
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

// findSUMBinary resolves the SUM executable in priority order:
//  1. SUM_PATH env override (explicit operator choice / volume mount),
//  2. a SUM uploaded through the web UI and persisted in the data dir,
//  3. a bundled/co-located SUM (local dev, Windows folder next to the .exe).
func findSUMBinary() string {
	if envPath := os.Getenv("SUM_PATH"); envPath != "" {
		if info, err := os.Stat(envPath); err == nil && !info.IsDir() {
			return envPath
		}
	}
	if p := persistedSUMPath(); p != "" {
		return p
	}
	return findBundledSUM()
}

// findBundledSUM looks for a SUM binary shipped alongside the app.
func findBundledSUM() string {
	candidates := []string{
		// Windows candidates
		"sum.exe",
		"./sum.exe",
		"./sum_2.15.0_Win_x86_64/sum.exe",
		"sum_2.15.0_Win_x86_64/sum.exe",
		"sum_tool/sum.exe",
		"../sum_2.15.0_Win_x86_64/sum.exe",

		// Linux candidates
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

// sumInstallMu serializes SUM installs so two concurrent uploads cannot race
// while replacing the persisted toolchain.
var sumInstallMu sync.Mutex

// sumDataDir is the persistent directory where an uploaded SUM toolchain is
// stored. In Docker it is a mounted volume (SUM_DATA_DIR=/app/data) so the
// installed SUM survives image rebuilds/updates. For the standalone .exe it
// defaults to a "sum_data" folder next to the working directory.
func sumDataDir() string {
	if d := os.Getenv("SUM_DATA_DIR"); d != "" {
		return d
	}
	return "sum_data"
}

func sumPathRecordFile() string {
	return filepath.Join(sumDataDir(), "sumpath.txt")
}

// persistedSUMPath returns the path of a previously uploaded SUM binary, or ""
// if none is recorded or the recorded file has gone missing.
func persistedSUMPath() string {
	data, err := os.ReadFile(sumPathRecordFile())
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(data))
	if p == "" {
		return ""
	}
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		return ""
	}
	return p
}

func recordSUMPath(p string) error {
	if err := os.MkdirAll(sumDataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(sumPathRecordFile(), []byte(p), 0o644)
}

// sumBinaryName is the SUM executable file name expected for the current OS.
func sumBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sum.exe"
	}
	return "sum"
}

type SUMStatusResponse struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Source    string `json:"source"` // env | uploaded | bundled | none
	Expected  string `json:"expected_binary"`
}

func sumStatus() SUMStatusResponse {
	st := SUMStatusResponse{Expected: sumBinaryName()}
	if envPath := os.Getenv("SUM_PATH"); envPath != "" {
		if info, err := os.Stat(envPath); err == nil && !info.IsDir() {
			st.Installed, st.Path, st.Source = true, envPath, "env"
			return st
		}
	}
	if p := persistedSUMPath(); p != "" {
		st.Installed, st.Path, st.Source = true, p, "uploaded"
		return st
	}
	if p := findBundledSUM(); p != "" {
		st.Installed, st.Path, st.Source = true, p, "bundled"
		return st
	}
	st.Source = "none"
	return st
}

func handleSUMStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sumStatus())
}

// handleSUMUpload accepts a SUM archive (.zip or .tar.gz), extracts it into the
// persistent data dir, locates the SUM binary for the current OS and records it
// so subsequent activations use it. The install survives container updates
// because the data dir is a mounted volume.
func handleSUMUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxUpload = 300 << 20 // 300 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSONError(w, "Не удалось прочитать загрузку (возможно, файл слишком большой): "+err.Error())
		return
	}

	file, hdr, err := r.FormFile("archive")
	if err != nil {
		writeJSONError(w, "Файл архива не найден в запросе (ожидается поле 'archive')")
		return
	}
	defer file.Close()

	sumInstallMu.Lock()
	defer sumInstallMu.Unlock()

	dataDir := sumDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		writeJSONError(w, "Не удалось создать каталог данных: "+err.Error())
		return
	}

	// Buffer the upload to a temp file so ZIP (which needs random access) works.
	tmpFile, err := os.CreateTemp(dataDir, "upload-*.bin")
	if err != nil {
		writeJSONError(w, "Временный файл: "+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeJSONError(w, "Ошибка сохранения загрузки: "+err.Error())
		return
	}
	tmpFile.Close()

	staging, err := os.MkdirTemp(dataDir, "incoming-*")
	if err != nil {
		writeJSONError(w, "Каталог распаковки: "+err.Error())
		return
	}
	defer os.RemoveAll(staging)

	if err := extractArchive(tmpPath, staging); err != nil {
		writeJSONError(w, "Ошибка распаковки '"+hdr.Filename+"': "+err.Error())
		return
	}

	binRel := findBinaryInTree(staging)
	if binRel == "" {
		writeJSONError(w, fmt.Sprintf("В архиве не найден бинарник %q. Убедитесь, что это архив SUM для %s.", sumBinaryName(), runtime.GOOS))
		return
	}

	// Promote staging -> <dataDir>/sum, replacing any previous install.
	finalDir := filepath.Join(dataDir, "sum")
	if err := os.RemoveAll(finalDir); err != nil {
		writeJSONError(w, "Не удалось очистить прежний SUM: "+err.Error())
		return
	}
	if err := os.Rename(staging, finalDir); err != nil {
		writeJSONError(w, "Не удалось установить SUM: "+err.Error())
		return
	}

	binPath := filepath.Join(finalDir, binRel)
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binPath, 0o755)
	}
	if err := recordSUMPath(binPath); err != nil {
		writeJSONError(w, "Не удалось сохранить путь SUM: "+err.Error())
		return
	}

	st := sumStatus()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "SUM успешно установлен и будет использоваться для активации.",
		"installed": st.Installed,
		"path":      st.Path,
		"source":    st.Source,
	})
}

func writeJSONError(w http.ResponseWriter, msg string) {
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": msg})
}

// extractArchive detects the archive format (.zip or .tar.gz) by magic bytes
// and unpacks it into dest.
func extractArchive(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	magic := make([]byte, 4)
	n, _ := io.ReadFull(f, magic)
	magic = magic[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	switch {
	case len(magic) >= 4 && magic[0] == 'P' && magic[1] == 'K' && magic[2] == 0x03 && magic[3] == 0x04:
		return extractZip(archivePath, dest)
	case len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		return extractTarGz(f, dest)
	default:
		return fmt.Errorf("неподдерживаемый формат архива (ожидается .zip или .tar.gz)")
	}
}

func extractZip(archivePath, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, zf := range zr.File {
		info := zf.FileInfo()
		open := func() (io.ReadCloser, error) { return zf.Open() }
		if err := writeArchiveEntry(dest, zf.Name, info.Mode(), info.IsDir(), open); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		info := hdr.FileInfo()
		open := func() (io.ReadCloser, error) { return io.NopCloser(tr), nil }
		if err := writeArchiveEntry(dest, hdr.Name, info.Mode(), info.IsDir(), open); err != nil {
			return err
		}
	}
	return nil
}

// writeArchiveEntry writes a single archive entry under dest, guarding against
// Zip-Slip path traversal and skipping symlinks/irregular files.
func writeArchiveEntry(dest, name string, mode fs.FileMode, isDir bool, open func() (io.ReadCloser, error)) error {
	target := filepath.Join(dest, name)
	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
		return fmt.Errorf("небезопасный путь в архиве: %s", name)
	}

	if isDir {
		return os.MkdirAll(target, 0o755)
	}
	// Only extract regular files; ignore symlinks, devices, etc.
	if !mode.IsRegular() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	rc, err := open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	const maxPerFile = 200 << 20 // cap any single file at 200 MiB
	if _, err := io.Copy(out, io.LimitReader(rc, maxPerFile)); err != nil {
		return err
	}
	return nil
}

// findBinaryInTree returns the path (relative to root) of the SUM binary for
// the current OS, or "" if not present in the extracted tree.
func findBinaryInTree(root string) string {
	want := sumBinaryName()
	var found string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), want) {
			if rel, relErr := filepath.Rel(root, path); relErr == nil {
				found = rel
			}
		}
		return nil
	})
	return found
}
