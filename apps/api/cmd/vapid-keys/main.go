// Command vapid-keys prints a fresh VAPID keypair for Web Push.
//
// Run once per environment via `make vapid-keys` and paste the output into
// .env as VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY. Never commit real values.
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate VAPID keys:", err)
		os.Exit(1)
	}
	fmt.Println("VAPID_PUBLIC_KEY=" + pub)
	fmt.Println("VAPID_PRIVATE_KEY=" + priv)
}
