//go:build embedpayload

package main

import _ "embed"

//go:embed payload.zip
var embeddedPayloadZip []byte

func init() {
	payloadZip = embeddedPayloadZip
}
