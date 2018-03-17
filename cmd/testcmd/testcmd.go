/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const use = "testcmd"

var (
	globalFlagTarget string
	localFlagTarget  string
)

func init() {
	pflag.StringVar(&globalFlagTarget, "global-flag", globalFlagTarget, "globally-registered flag")
}

func NewLocalFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet(use, pflag.ContinueOnError)
	fs.StringVar(&localFlagTarget, "local-flag", localFlagTarget, "locally-registered flag")
	return fs
}

func NewTestCmd() *cobra.Command {
	localFlagSet := NewLocalFlagSet()
	cmd := &cobra.Command{
		Use:                use,
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			// parse our local flag set
			if err := localFlagSet.Parse(args); err != nil {
				cmd.Usage()
				fatal(err)
			}
			// --help
			help, err := localFlagSet.GetBool("help")
			if err != nil {
				fatal(fmt.Errorf(`"help" flag is non-bool, programmer error, please correct`))
			}
			if help {
				cmd.Help()
				return
			}
			// print the flag values
			fmt.Printf("globalFlagTarget: %q\n", globalFlagTarget)
			fmt.Printf("localFlagTarget: %q\n", localFlagTarget)
		},
	}
	localFlagSet.BoolP("help", "h", false, fmt.Sprintf("help for %s", cmd.Name()))
	// Cobra still needs to be aware of our flag set to generate usage and help text
	cmd.Flags().AddFlagSet(localFlagSet)
	return cmd
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func main() {
	cmd := NewTestCmd()
	if err := cmd.Execute(); err != nil {
		fatal(err)
	}
}
