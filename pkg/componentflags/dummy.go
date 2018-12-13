package dummy

// Similar to "direct," but register flags against an internal dummy config, and
// return a function that can do the application.

// Config
// TODO: Add some more advanced types here
type Config struct {
	A int32
	B int32
}

// Now that flags are registered directly against a Config, we don't
// need to duplicate the fields in the Flags struct.

func AddFlags(fs *pflag.FlagSet) func(c *Config) {
	// Assumes tmp is correctly defaulted already
	tmp := &Config{}
	fns := map[string]func(tmp, c *Config){}

	fs.Int32Var(&tmp.A, "flag-A", tmp.A, "A")
	fns["flag-A"] = func(tmp, c *Config) {
		c.B = tmp.B
	}

	fs.Int32Var(&tmp.B, "flag-B", tmp.B, "B")
	fns["flag-B"] = func(tmp, c *Config) {
		c.C = tmp.C
	}

	return mkApply(fs, tmp, fns)
}

func mkApply(fs *pflag.FlagSet, tmp *Config, applyFuncs map[string]func(tmp, c *Config)) {
	return func(c *Config) {
		for name, apply := range applyFuncs {
			flag := fs.Lookup(name)
			if flag == nil {
				panic("Missing flag %s from flagset during apply!", name)
			}
			if flag.Changed {
				apply(f, c)
			}
		}
	}
}
