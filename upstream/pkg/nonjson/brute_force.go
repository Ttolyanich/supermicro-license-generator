package nonjson

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	netinternal "github.com/zsrv/supermicro-product-key/pkg/net"
	"github.com/zsrv/supermicro-product-key/pkg/oob"
)

// BruteForceMACAddress finds the MAC address associated with an encrypted
// product key. The MAC address can then be used to decrypt the key.
func BruteForceMACAddress(encodedProductKey string) (string, error) {
	return BruteForceMACAddressContext(context.Background(), encodedProductKey)
}

// BruteForceMACAddressContext behaves like BruteForceMACAddress but stops
// searching as soon as ctx is cancelled, returning ctx.Err(). This bounds the
// CPU cost of a search so a cancelled or timed-out request does not keep
// worker goroutines busy. The channels are buffered so cancelled workers never
// block on send and are free to return.
func BruteForceMACAddressContext(ctx context.Context, encodedProductKey string) (string, error) {
	blocks := oob.SupermicroMACAddressBlocks
	result := make(chan netinternal.HardwareAddr, 1)
	done := make(chan bool, len(blocks))

	brute := func(macBlock [3]byte) {
		log.Debug().Msgf("searching mac address block %X", macBlock)

		mac := make(netinternal.HardwareAddr, 6)
		for one := 0; one <= 255; one++ {
			// Check for cancellation once per outer iteration to keep the
			// search responsive without per-key overhead.
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

					if _, err := ParseEncodedProductKey(encodedProductKey, mac); err != nil {
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
