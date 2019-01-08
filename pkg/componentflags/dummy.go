package dummy

// Similar to "direct," but register flags against an internal dummy config, and
// return a function that can do the application.

// Now that flags are registered directly against a Config, we don't
// need to duplicate the fields in the Flags struct.

// The common cases should be easy, but the uncommon cases should still be possible.

// Can we abstract away apply behavior as well?
// Since it's a Config -> Config function, field names
// match up.
// What if the flag registration function could accept merge behavior?
// Is there a way to do this without losing type safety?

// Since we always make the default the same as the current value of the field, we can
// omit that too...

// TODO(mtaufen): probably keep usage string at the end of the arguments
// For example, could we pass a field path in here and reflect on it to get the value and a pointer?
// TODO: How can we get rid of the need for users to typecast in the apply func that they write? (HINT HINT MACROS OR GENERICS WOULD BE NICE!)

// Config is the internal representation of a component config
type Config struct {
	A int32
	B []string
}

// NewConfig returns a Config with default values set
func NewConfig() *Config {
	c := &Config{}
	Default(c)
	return c
}

// Default applies defautls to a Config, if values are not already set
func Default(c *Config) {
	if c.A == 0 {
		c.A = 1
	}
	if c.B == nil {
		c.B = []string{"foo"}
	}
}

// ConfigFlagSet will register an applyFunc every time a flag is registered, and
// can apply all of the applyFuncs to a Config, using values parsed into the tmp Config.
type ConfigFlagSet struct {
	tmp        interface{}
	fs         *pflag.FlagSet
	applyFuncs map[string]func(c *Config)
}

// tmp should already contain the default values
func NewConfigFlagSet(fs *pflag.FlagSet, tmp interface{}) *ConfigFlagSet {
	return &ConfigFlagSet{tmp: tmp, fs: fs}
}

func (fs *ConfigFlagSet) Int32Var(name string, usage string, target func(c interface{}) *int32, merge func(tmp, c int32) int32) {
	ptmp := target(fs.tmp)
	fs.fs.Int32Var(ptmp, name, *ptmp, usage)
	// This direct application may be a reasonable default
	fs.applyFuncs[name] = func(c interface{}) {
		pc := target(c)
		if merge == nil {
			*pc = *ptmp
		} else {
			*pc = merge(*ptmp, *pc)
		}
	}
}

func (fs *ConfigFlagSet) StringSliceVar(name string, usage string,
	target func(c interface{}) *[]string, merge func(tmp, c []string) []string) {
	ptmp := target(fs.tmp)
	fs.fs.StringSliceVar(ptmp, name, *ptmp, usage)
	// This direct application may be a reasonable default
	fs.applyFuncs[name] = func(c interface{}) {
		pc := target(c)
		if merge == nil {
			*pc = []string{}
			for _, v := range *ptmp {
				*pc = append(*pc, v)
			}
		} else {
			// TODO(mtaufen): also consider copy before calling merge here, to avoid side-effects
			*pc = merge(*ptmp, *pc)
		}
	}
}

func (fs *ConfigFlagSet) Apply(c interface{}) {
	for name, apply := range applyFuncs {
		if fs.fs.Changed(name) {
			apply(c)
		}
	}
}

func AddConfigFlags(fs *pflag.FlagSet) func(c interface{}) {
	tmp = NewConfig()
	cfs := NewConfigFlagSet(fs)

	cfs.Int32Var("a-flag", "", func(c interface{}) *int32 {
		return &c.(*Config).A
	}, nil)

	cfs.StringSliceVar("b-flag", "", func(c interface{}) *[]string {
		return &c.(*Config).B
	}, func(a, b []string) []string {
		s := []string{}
		s = append(s, a...)
		s = append(s, b...)
		return s
	})

	return cfs.Apply
}
