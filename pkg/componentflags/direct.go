package direct

// Simple, manual apply fn. for flags struct -> config struct
// This puts the flag surface area in one explicit, highly visible place.
// It's also pretty easy to understand.
// Easy to customize/override merge behavior in the map (just change the func).
// One downside is it requires you to track fs, f, and c outside these helpers.

// We might even want to say that any 3rd party lib flags must also be in this Flags struct,
// and then internally apply them to those libs as necessary.

// Cleanly separate the Parse and Apply steps for all configuration.

// Config
// TODO: Add some more advanced types here
type Config struct {
	A int32
	B int32
}

// Flags that overlap config
type Flags struct {
	A int32
	B int32
}

func (f *Flags) AddFlags(fs *pflag.FlagSet) {
	fs.Int32Var(&f.A, "flag-A", f.A, "A")
	fs.Int32Var(&f.B, "flag-B", f.B, "B")
}

func ApplyFlagsToConfig(fs *pflag.FlagSet, f *Flags, c *Config) {
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

var (
	applyFuncs = map[string]func(f *Flags, c *Config){
		"flag-A": func(f *Flags, c *Config) {
			c.B = f.B
		},
		"flag-B": func(f *Flags, c *Config) {
			c.C = f.C
		},
	}
)
