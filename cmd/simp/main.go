package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/auth"
	"github.com/busthorne/simp/books"
	"github.com/busthorne/simp/cable"
	"github.com/busthorne/simp/config"
	"github.com/fsnotify/fsnotify"
	"github.com/gofiber/fiber/v2/log"
)

var (
	// diff             = flag.Bool("diff", false, "if configured, diff from instructions")
	configure        = flag.Bool("configure", false, "interactive configuration wizard")
	daemon           = flag.Bool("daemon", false, "run as daemon")
	vim              = flag.Bool("vim", false, "vim mode")
	historypath      = flag.Bool("historypath", false, "display history path per current location")
	interactive      = flag.Bool("i", false, "interactive mode")
	verbose          = flag.Bool("v", false, "verbose output")
	debug            = flag.Bool("vv", false, "very verbose (debug) output")
	tracing          = flag.Bool("vvv", false, "very, very verbose (trace) output")
	nos              = flag.Bool("nos", false, "disable streaming")
	lessThan         = flag.Int("lt", 0, "less than this many tokens")
	temperature      = flag.Float64("t", -1, "temperature")
	topP             = flag.Float64("p", -1, "top_p sampling")
	frequencyPenalty = flag.Float64("fp", -1, "frequency penalty")
	presencePenalty  = flag.Float64("pp", -1, "presence penalty")

	model     string
	anthology string // winning history path
	cfg       *config.Config
	cab       *cable.Cable

	bg = context.Background()
)

type stimulus chan struct{}

func main() {
	log.SetLevel(log.LevelInfo)

	wave() // the flags

	conflicts := mutuallyExclusive(
		"configure",
		"daemon",
		"historypath",
		"i",
	)
	if conflicts != nil {
		stderr("mutually exclusive flags:", strings.Join(conflicts, ", "))
		exit(1)
	}

	if err := setup(); err != nil {
		stderr("simp:", err)
		exit(1)
	}

	switch {
	case *historypath:
		fmt.Println(anthology)
		return
	case *daemon:
		if *verbose {
			log.SetLevel(log.LevelInfo)
		}
		if *debug {
			log.SetLevel(log.LevelDebug)
		}
		if *tracing {
			log.SetLevel(log.LevelTrace)
		}
		w, err := fsnotify.NewWatcher()
		if err != nil {
			stderr("failed to create watcher:", err)
			exit(1)
		}
		defer w.Close()
		w.Add(path.Join(simp.Path, "config"))

		reload := make(stimulus)
		go func() {
			for e := range w.Events {
				if e.Op == fsnotify.Write && strings.HasSuffix(e.Name, ".hcl") {
					reload <- struct{}{}
				}
			}
		}()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		for {
			f := listen()
			for {
				select {
				case <-reload:
					log.Info("config changed, reloading")
					cfg.ClearCache()
					auth.ClearCache()
				case <-sig:
					return
				}
				if f != nil {
					if err := f.Shutdown(); err != nil {
						log.Error("failed to shutdown:", err)
						return
					}
					f = nil
					log.Info("shutdown")
				}
				if err := setup(); err != nil {
					log.Error("failed to setup:", err)
					continue
				}
				break
			}
		}
	}
	defer saveHistory()

	if *interactive {
		stderr(interactiveHelp)
	}
	for {
		switch err := promptComplete(); err {
		case nil:
		case io.EOF:
			// so that the reader doesn't have to wait for history
			os.Stdout.Close()
			return
		default:
			log.Error("failed to complete:", err)
			exit(1)
		}
	}
}

func wave() {
	// allow model to be specified ahead of flags
	if args := os.Args; len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		model = args[1]
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	flag.Parse()
	if *configure {
		wizard()
		exit(1)
	}
	// positional control flags
	for k, arg := range flag.Args() {
		j, err := strconv.ParseFloat(arg, 32)
		if err != nil {
			if k == 0 {
				model = arg
				continue
			}
			flag.Usage()
			exit(1)
		}
		switch k {
		case 0:
			*temperature = j
		case 1:
			*lessThan = int(j)
		case 2:
			*topP = j
		case 3:
			*frequencyPenalty = j
		case 4:
			*presencePenalty = j
		}
		if err != nil {
			flag.Usage()
			exit(1)
		}
	}
}

func setup() error {
	p := path.Join(simp.Path, "config")
	c, err := config.ParsePath(p)
	if err != nil {
		for _, d := range c.Diagnostics {
			for _, err := range d.Errs() {
				stderr(err)
			}
		}
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	cfg = c

	if model == "" {
		if cfg.Default.Model == "" {
			return errors.New("no default model")
		}
		model = cfg.Default.Model
	}

	// get working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// winning path for history
	anthology = history(cfg.History, wd)
	if anthology != "" {
		if err := os.MkdirAll(anthology, 0755); err != nil {
			return fmt.Errorf("history path %s per working directory: %w", anthology, err)
		}
	}

	// open the database
	if err := books.Open(path.Join(simp.Path, "books.db3")); err != nil {
		stderr("failed to open books:", err)
		exit(1)
	}
	return nil
}

var errNoKeyring = errors.New("no keyring")

func keyringFor(p config.Provider, c *config.Config) (keyring.Keyring, error) {
	if c == nil {
		c = cfg
	}
	var k config.Auth
	for _, a := range c.Auth {
		if a.Type != "keyring" {
			continue
		}
		if p.Keyring != "" && p.Keyring == a.Name {
			return auth.NewKeyring(a, &p)
		}
		if a.Default {
			k = a
		}
	}
	if k.Type == "" {
		return nil, errNoKeyring
	}
	return auth.NewKeyring(k, &p)
}

func stderr(a ...interface{}) {
	fmt.Fprintln(os.Stderr, a...)
}

func stderrf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
}

func exit(code int) {
	os.Exit(code)
}

func coalesce(a ...any) (k float32) {
	for _, v := range a {
		switch v := v.(type) {
		case *float32:
			if v == nil {
				continue
			}
			k = *v
		case *float64:
			if v == nil {
				continue
			}
			k = float32(*v)
		}
		if k < 0 {
			continue
		}
		return
	}
	return 0
}

// mutuallyExclusive accepts the flags of which only one can be set,
// and returns the names of the flags that were set, if there's
// more than one such flag
func mutuallyExclusive(flags ...string) []string {
	var set []string
	flag.Visit(func(f *flag.Flag) {
		for _, flag := range flags {
			if f.Name == flag {
				set = append(set, "-"+f.Name)
			}
		}
	})
	if len(set) <= 1 {
		return nil
	}
	return set
}
