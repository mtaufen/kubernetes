/*
Copyright 2017 The Kubernetes Authors.

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

package nodeconfig

import (
	"fmt"

	apiv1 "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/apis/componentconfig/validation"
)

// badRollback makes an entry in the bad-config-tracking file for `uid` with `reason`, and then
// returns the result of rolling back to the last-known-good config.
// If filesystem issues prevent marking the config bad or rolling back, a panic occurs.
func (cc *NodeConfigController) badRollback(uid, reason, detail string) *componentconfig.KubeletConfiguration {
	if len(detail) > 0 {
		detail = fmt.Sprintf(", %s", detail)
	}
	errorf(fmt.Sprintf("%s%s", reason, detail))
	cc.markBadConfig(uid, reason)
	return cc.lkgRollback(reason, apiv1.ConditionFalse)
}

// lkgRollback loads, verifies, parses, validates, and returns the last-known-good configuration,
// and updates the `cc.configOK` condition regarding the `cause` of the rollback. The `status` argument
// indicates whether the rollback is due to a known-bad config (apiv1.ConditionFalse) or because the system
// couldn't sync a configuration (statusUnknown).
// If the `defaultConfig` or `initConfig` is the last-known-good, the cached copies are immediately returned.
// If the last-known-good fails any of load, verify, parse, or validate,
// attempts to report the associated ConfigOK condition and a panic occurs.
// If filesystem issues prevent returning the last-known-good configuration, a panic occurs.
func (cc *NodeConfigController) lkgRollback(cause string, status apiv1.ConditionStatus) *componentconfig.KubeletConfiguration {
	infof("rolling back to last-known-good config")

	// if lkgUID indicates the default should be used, return initConfig or defaultConfig
	lkgUID := cc.lkgUID()
	if len(lkgUID) == 0 {
		if cc.initConfig != nil {
			cc.setConfigOK(lkgInitMessage, cause, status)
			return cc.initConfig
		}
		cc.setConfigOK(lkgDefaultMessage, cause, status)
		return cc.defaultConfig
	}

	// load
	toVerify, err := cc.loadCheckpoint(lkgSymlink)
	if err != nil {
		cause := fmt.Sprintf(lkgFailLoadReasonFmt, lkgUID)
		cc.panicSyncConfigOK(cause)
		panicf("%s, error: %v", cause, err)
	}

	// verify
	toParse, err := toVerify.verify()
	if err != nil {
		cause := fmt.Sprintf(lkgFailVerifyReasonFmt, lkgUID)
		cc.panicSyncConfigOK(cause)
		panicf("%s, error: %v", cause, err)
	}

	// parse
	lkg, err := toParse.parse()
	if err != nil {
		cause := fmt.Sprintf(lkgFailParseReasonFmt, lkgUID)
		cc.panicSyncConfigOK(cause)
		panicf("%s, error: %v", cause, err)
	}

	// validate
	if err := validation.ValidateKubeletConfiguration(lkg); err != nil {
		cause := fmt.Sprintf(lkgFailValidateReasonFmt, lkgUID)
		cc.panicSyncConfigOK(cause)
		panicf("%s, error: %v", cause, err)
	}

	// update the ConfigOK status
	cc.setConfigOK(fmt.Sprintf(lkgRemoteMessageFmt, lkgUID), cause, status)
	return lkg
}
