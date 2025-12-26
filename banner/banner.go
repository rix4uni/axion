package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.1"

func PrintVersion() {
	fmt.Printf("Current axion version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
                _             
  ____ _ _  __ (_)____   ____ 
 / __  /| |/_// // __ \ / __ \
/ /_/ /_>  < / // /_/ // / / /
\__,_//_/|_|/_/ \____//_/ /_/                             
`
	fmt.Printf("%s\n%40s\n\n", banner, "Current axion version "+version)
}
