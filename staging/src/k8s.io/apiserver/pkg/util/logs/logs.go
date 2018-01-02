/*
Copyright 2014 The Kubernetes Authors.

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

package logs

import (
	"flag"
	"log"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/wait"
	flagutil "k8s.io/apiserver/pkg/util/flag"
)

const (
	logFlushFreqFlagName = "log-flush-frequency"
	logFlushFreqFlagHelp = "Maximum number of seconds between log flushes"
)

var logFlushFreq = pflag.Duration(logFlushFreqFlagName, 5*time.Second, logFlushFreqFlagHelp)

// TODO(thockin): This is temporary until we agree on log dirs and put those into each cmd.
func init() {
	flag.Set("logtostderr", "true")
}

// AddFlags registers this package's flags on arbitrary FlagSets, such that they point to the
// same value as the global flags.
// If fake is true, will register the flag name, but point it at a value with a noop Set implementation.
func AddFlags(fs *pflag.FlagSet, fake bool) {
	if fake {
		fs.Var(flagutil.NoOp{}, logFlushFreqFlagName, logFlushFreqFlagHelp)
		return
	}
	f := pflag.Lookup(logFlushFreqFlagName)
	fs.Var(f.Value, logFlushFreqFlagName, logFlushFreqFlagHelp)
}

// GlogWriter serves as a bridge between the standard log package and the glog package.
type GlogWriter struct{}

// Write implements the io.Writer interface.
func (writer GlogWriter) Write(data []byte) (n int, err error) {
	glog.Info(string(data))
	return len(data), nil
}

// InitLogs initializes logs the way we want for kubernetes.
func InitLogs() {
	log.SetOutput(GlogWriter{})
	log.SetFlags(0)
	// The default glog flush interval is 30 seconds, which is frighteningly long.
	go wait.Until(glog.Flush, *logFlushFreq, wait.NeverStop)
}

// FlushLogs flushes logs immediately.
func FlushLogs() {
	glog.Flush()
}

// NewLogger creates a new log.Logger which sends logs to glog.Info.
func NewLogger(prefix string) *log.Logger {
	return log.New(GlogWriter{}, prefix, 0)
}
