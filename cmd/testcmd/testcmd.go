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
	fs.StringVar(&localFlagTarget, "test", localFlagTarget, "locally-registered flag")
	return fs
}

func NewTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: use,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("globalFlagTarget: %q\n", globalFlagTarget)
			fmt.Printf("localFlagTarget: %q\n", localFlagTarget)
		},
	}
	cmd.Flags().AddFlagSet(NewLocalFlagSet())
	return cmd
}

func main() {
	cmd := NewTestCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
