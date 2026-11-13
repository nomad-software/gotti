// termite is a GPU-accelerated terminal emulator for Linux.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"

	"termite/internal/appicon"
	"termite/internal/config"
	"termite/internal/desktop"
	"termite/internal/ui"
)

func main() {
	cfgPath := flag.String("config", config.Path(), "configuration file")
	layout := flag.String("layout", "", "open a saved layout")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile to this file")
	memProfile := flag.String("memprofile", "", "write a heap profile on exit")
	pprofAddr := flag.String("pprof", "", "serve net/http/pprof on this address (e.g. localhost:6060)")
	writeConfig := flag.Bool("write-config", false, "write the default config (JSON) and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: termite [flags] [-- command args...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *writeConfig {
		if err := config.WriteSample(*cfgPath); errors.Is(err, fs.ErrExist) {
			fatal(fmt.Errorf("%s already exists", *cfgPath))
		} else if err != nil {
			fatal(err)
		}
		fmt.Println("wrote", *cfgPath)
		return
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fatal(err)
		}
		defer pprof.StopCPUProfile()
	}
	if *pprofAddr != "" {
		go func() { fmt.Fprintln(os.Stderr, http.ListenAndServe(*pprofAddr, nil)) }()
	}

	cfg, cfgErr := config.Load(*cfgPath)
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "termite:", cfgErr)
	}
	var lf *ui.LayoutFile
	if *layout != "" {
		var err error
		if lf, err = ui.LoadLayout(*layout); err != nil {
			fatal(err)
		}
	}

	ui.NewApplication()
	qtcore.QCoreApplicationSetApplicationName("termite")
	// No application display name is set: Qt would append it to every
	// window title, which shows the focused pane's directory alone.

	// Under Wayland this is the name a window goes by, which a desktop
	// matches to the desktop entry of that name for its icon and its place
	// in the task bar.
	qtgui.QGuiApplicationSetDesktopFileName(desktop.Name)
	qtgui.QGuiApplicationSetWindowIcon(ui.AppIcon())

	// The desktop is given an entry for the program, which starts the copy
	// that is running now, wherever it is and whatever it is called, so that
	// it can be started from the desktop's own menus and kept in a dock or
	// sidebar.
	if os.Getenv("TERMITE_NO_DESKTOP_ENTRY") == "" {
		if err := desktop.Install(appicon.PNG); err != nil {
			fmt.Fprintln(os.Stderr, "termite: the desktop entry could not be written:", err)
		}
	}

	qute.Main(func() {
		app := ui.NewApp(cfg, *cfgPath, cfgErr, flag.Args())
		app.NewWindow(lf)
	})
	ui.FlushFrameStats()

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			fatal(err)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fatal(err)
		}
		f.Close()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "termite:", err)
	os.Exit(1)
}
