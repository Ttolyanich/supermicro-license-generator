package oob

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	netinternal "github.com/zsrv/supermicro-product-key/pkg/net"
)

// SupermicroMACAddressBlocks contains MAC address blocks
// that have been assigned to Supermicro.
//
// https://www.wireshark.org/download/automated/data/manuf
var SupermicroMACAddressBlocks = [][3]byte{
	{0x00, 0x25, 0x90},
	{0x00, 0x30, 0x48},
	{0x0c, 0xc4, 0x7a},
	{0x3c, 0xec, 0xef},
	{0x7c, 0xc2, 0x55},
	{0x90, 0x5a, 0x08},
	{0xac, 0x1f, 0x6b},
}

// BruteForceMACAddress returns the MAC address associated with the
// product key, or an error if one occurs or the MAC address was not found.
func BruteForceMACAddress(productKey string) (string, error) {
	return BruteForceMACAddressContext(context.Background(), productKey)
}

// BruteForceMACAddressContext behaves like BruteForceMACAddress but stops
// searching as soon as ctx is cancelled, returning ctx.Err(). This lets a
// caller bound the CPU cost of a search (e.g. a request timeout) instead of
// letting the worker goroutines run to completion. The channels are buffered
// so cancelled workers never block on send and are free to return.
func BruteForceMACAddressContext(ctx context.Context, productKey string) (string, error) {
	pk, err := ParseProductKey(productKey)
	if err != nil {
		return "", err
	}

	blocks := SupermicroMACAddressBlocks
	result := make(chan netinternal.HardwareAddr, 1)
	done := make(chan bool, len(blocks))

	brute := func(macBlock [3]byte) {
		log.Debug().Msgf("searching mac address block %X", macBlock)

		mac := make(netinternal.HardwareAddr, 6)
		for one := 0; one <= 255; one++ {
			// Check for cancellation once per outer iteration (~65k inner
			// iterations), which keeps the search responsive within
			// milliseconds without measurable per-key overhead.
			select {
			case <-ctx.Done():
				done <- true
				return
			default:
			}

			for two := 0; two <= 255; two++ {
				for three := 0; three <= 255; three++ {
					mac[0] = macBlock[0]
					mac[1] = macBlock[1]
					mac[2] = macBlock[2]
					mac[3] = byte(one)
					mac[4] = byte(two)
					mac[5] = byte(three)

					generatedKey, _ := EncodeOOBProductKey(mac)
					if !generatedKey.Equal(pk) {
						continue
					}

					m := make(netinternal.HardwareAddr, len(mac))
					copy(m, mac)
					result <- m
					return
				}
			}
		}

		log.Debug().Msgf("finished searching mac address block %X with no matches", macBlock)
		done <- true
	}

	for _, macBlock := range blocks {
		go brute(macBlock)
	}

	for range blocks {
		select {
		case resultMAC := <-result:
			return resultMAC.String(), nil
		case <-done:
			continue
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	return "", errors.New("could not find a matching mac address")
}
