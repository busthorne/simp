package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/busthorne/keyring"
	"github.com/busthorne/simp"
	"github.com/busthorne/simp/auth"
	"github.com/busthorne/simp/config"
)

var (
	// diff             = flag.Bool("diff", false, "if configured, diff from instructions")
	configure        = flag.Bool("configure", false, "interactive configuration wizard")
	apikey           = flag.Bool("apikey", false, "configure provider apikey in keyring")
	daemon           = flag.Bool("daemon", false, "run as daemon")
	vim              = flag.Bool("vim", false, "vim mode")
	historypath      = flag.Bool("historypath", false, "display history path per current location")
	interactive      = flag.Bool("i", false, "interactive mode")
	lessThan         = flag.Int("lt", 0, "less than this many tokens")
	temperature      = flag.Float64("t", 0.7, "temperature")
	topP             = flag.Float64("p", 1.0, "top_p sampling")
	frequencyPenalty = flag.Float64("fp", 0, "frequency penalty")
	presencePenalty  = flag.Float64("pp", 0, "presence penalty")

	model     string
	ws        string
	anthology string // winning history path
	cfg       *config.Config
	cable     simp.Cable

	bg = context.Background()
)

func main() {
	wave() // the flags
	setup()
	switch {
	case *apikey:
		wizardApikey()
		return
	case *historypath:
		fmt.Println(anthology)
		return
	case *daemon:
		gateway()
		return
	}
	defer saveHistory()
	for {
		switch err := promptComplete(); err {
		case nil:
		case io.EOF:
			// so that the reader doesn't have to wait for history
			os.Stdout.Close()
			return
		default:
			stderr(err)
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
		return
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

func setup() {
	p := path.Join(simp.Path, "config")
	c, err := config.ParsePath(p)
	if err != nil {
		stderr("simp:", err)
		for _, d := range c.Diagnostics {
			for _, err := range d.Errs() {
				stderr(err)
			}
		}
		exit(1)
	}
	if err := c.Validate(); err != nil {
		stderrf("%s: %v\n", p, err)
		exit(1)
	}
	cfg = c
	if model == "" {
		if cfg.Default.Model == "" {
			stderr("no default model")
			exit(1)
		}
		model = cfg.Default.Model
	}
	// get working directory
	wd, err := os.Getwd()
	if err != nil {
		stderr("simp:", err)
		exit(1)
	}
	// winning path for history
	anthology = history(cfg.History, wd)
	if anthology != "" {
		if err := os.MkdirAll(anthology, 0755); err != nil {
			stderrf("history path %s per working directory: %v", anthology, err)
			exit(1)
		}
	}
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

func coalesce32(a ...*float64) float32 {
	for _, v := range a {
		if v == nil {
			continue
		}
		return float32(*v)
	}
	return 0
}
