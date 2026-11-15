// Command doormutex-streamdeck is the doormutex Stream Deck plugin. The
// Stream Deck software (Elgato's app, or OpenDeck on Linux) starts it with
// -port, -pluginUUID, -registerEvent and -info; it is not meant to be run
// by hand, apart from -write-icons when building the plugin bundle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/padalton/doormutex/companion/internal/streamdeck"
)

var version = "dev"

func main() {
	var a streamdeck.Args
	flag.IntVar(&a.Port, "port", 0, "WebSocket port of the Stream Deck software")
	flag.StringVar(&a.PluginUUID, "pluginUUID", "", "this plugin instance's UUID")
	flag.StringVar(&a.RegisterEvent, "registerEvent", "registerPlugin", "event to register with")
	flag.StringVar(&a.Info, "info", "{}", "JSON about the software and devices")
	icons := flag.String("write-icons", "", "write the bundle's static pictures into this directory and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	switch {
	case *showVersion:
		fmt.Println(version)
		return
	case *icons != "":
		if err := streamdeck.WriteIcons(*icons); err != nil {
			log.Fatal(err)
		}
		return
	case a.Port == 0 || a.PluginUUID == "":
		fmt.Fprintln(os.Stderr, "doormutex-streamdeck is started by the Stream Deck software (Elgato or OpenDeck);")
		fmt.Fprintln(os.Stderr, "install the plugin bundle there, see streamdeck/README.md.")
		os.Exit(2)
	}
	log.SetFlags(log.LstdFlags)
	log.Printf("doormutex-streamdeck %s", version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := streamdeck.New(a).Run(ctx); err != nil {
		log.Fatal(err)
	}
}
