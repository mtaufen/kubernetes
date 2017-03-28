# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM alpine:3.5

RUN apk add --no-cache iptables

ADD non-masquerade-daemon /usr/bin/non-masquerade-daemon
RUN chmod +x /usr/bin/non-masquerade-daemon

ENTRYPOINT ["/usr/bin/non-masquerade-daemon"]