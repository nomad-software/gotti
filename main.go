// termite is a GPU-accelerated terminal emulator for Linux.
package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"

	qt "github.com/mappu/miqt/qt6"

	"termite/internal/appicon"
	"termite/internal/config"
	"termite/internal/desktop"
	"termite/internal/ui"
)

func init() {
	// Qt and GL calls must stay on the main OS thread.
	runtime.LockOSThread()
}

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
		if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
			fatal(err)
		}
		if _, err := os.Stat(*cfgPath); err == nil {
			fatal(fmt.Errorf("%s already exists", *cfgPath))
		}
		if err := os.WriteFile(*cfgPath, config.SampleConfig(), 0o644); err != nil {
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

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "termite:", err)
	}
	var lf *ui.LayoutFile
	if *layout != "" {
		if lf, err = ui.LoadLayout(*layout); err != nil {
			fatal(err)
		}
	}

	qt.QCoreApplication_SetAttribute(qt.AA_ShareOpenGLContexts)
	f := qt.NewQSurfaceFormat()
	f.SetRenderableType(qt.QSurfaceFormat__OpenGL)
	f.SetVersion(3, 3)
	f.SetProfile(qt.QSurfaceFormat__CoreProfile)
	f.SetDepthBufferSize(0)
	f.SetStencilBufferSize(0)
	f.SetSwapInterval(1) // vsync: at most one frame per refresh
	qt.QSurfaceFormat_SetDefaultFormat(f)

	qt.NewQApplication(os.Args)
	qt.QCoreApplication_SetApplicationName("termite")
	// Qt appends the display name to every window title (" — Termite").
	qt.QGuiApplication_SetApplicationDisplayName(desktop.DisplayName)
	// Under Wayland this is the name a window goes by, which a desktop
	// matches to the desktop entry of that name for its icon and its place
	// in the task bar.
	qt.QGuiApplication_SetDesktopFileName(desktop.Name)
	qt.QGuiApplication_SetWindowIcon(ui.AppIcon())

	// On Linux the desktop is given an entry for the program, which starts
	// the copy that is running now, wherever it is and whatever it is
	// called, so that it can be started from the desktop's own menus and
	// kept in a dock or sidebar.
	if runtime.GOOS == "linux" && os.Getenv("TERMITE_NO_DESKTOP_ENTRY") == "" {
		if err := desktop.Install(appicon.PNG); err != nil {
			fmt.Fprintln(os.Stderr, "termite: the desktop entry could not be written:", err)
		}
	}

	app := ui.NewApp(cfg, *cfgPath, flag.Args())
	app.NewWindow(lf)
	qt.QApplication_Exec()
	ui.FlushFrameStats()

	if *memProfile != "" {
		if f, err := os.Create(*memProfile); err == nil {
			runtime.GC()
			pprof.WriteHeapProfile(f)
			f.Close()
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "termite:", err)
	os.Exit(1)
}
