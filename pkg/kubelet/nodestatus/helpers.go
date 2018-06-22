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

package nodestatus

import (
	"k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/golang/glog"
)

// TODO(mtaufen): eventually tease apart sending events from the setter - perhaps have setters return
// a list of events that should be sent after all setters have run. May allow us to get better behavior wrt
// the preexisting "transaction" TODO (send events after status update succeeds), though I don't think we
// can ever guarantee a transaction between events and a status update (cross-object transactions are not supported by the API, etc.).
func recordNodeStatusEvent(recorder record.EventRecorder,
	nodeName string,
	nodeRef *v1.ObjectReference,
	eventType string,
	event string) {
	glog.V(2).Infof("Recording %s event message for node %s", event, nodeName)
	// TODO: This requires a transaction, either both node status is updated
	// and event is recorded or neither should happen, see issue #6055.
	recorder.Eventf(nodeRef, eventType, event, "Node %s status is now: %s", nodeName, event)
}
