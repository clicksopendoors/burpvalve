# Third-Party Notices

This notice bundle covers the shipped Burpvalve CLI binary dependency graph as
of the current `./cmd/burpvalve` build. It was derived from:

```sh
go list -deps -f '{{if not .Standard}}{{with .Module}}{{.Path}} {{.Version}}{{end}}{{end}}' ./cmd/burpvalve | sort -u
```

The project itself is licensed under the MIT License in `LICENSE`. The modules
below are redistributed through compiled release archives and require license
and notice preservation in release documentation or materials.

## Binary Dependency Graph

| Dependency | Version | License family | Required text included below |
| --- | --- | --- | --- |
| charm.land/bubbles/v2 | v2.1.0 | MIT | MIT notice |
| charm.land/bubbletea/v2 | v2.0.7 | MIT | MIT notice |
| charm.land/lipgloss/v2 | v2.0.4 | MIT | MIT notice |
| github.com/atotto/clipboard | v0.1.4 | BSD-3-Clause | BSD notice |
| github.com/charmbracelet/colorprofile | v0.4.3 | MIT | MIT notice |
| github.com/charmbracelet/ultraviolet | v0.0.0-20260525132238-948f4557a654 | MIT | MIT notice |
| github.com/charmbracelet/x/ansi | v0.11.7 | MIT | MIT notice |
| github.com/charmbracelet/x/term | v0.2.2 | MIT | MIT notice |
| github.com/charmbracelet/x/termios | v0.1.1 | MIT | MIT notice |
| github.com/charmbracelet/x/windows | v0.2.2 | MIT | MIT notice |
| github.com/clipperhouse/displaywidth | v0.11.0 | MIT | MIT notice |
| github.com/clipperhouse/uax29/v2 | v2.7.0 | MIT | MIT notice |
| github.com/lucasb-eyer/go-colorful | v1.4.0 | MIT | MIT notice |
| github.com/mattn/go-runewidth | v0.0.23 | MIT | MIT notice |
| github.com/muesli/cancelreader | v0.2.2 | MIT | MIT notice |
| github.com/rivo/uniseg | v0.4.7 | MIT | MIT notice |
| github.com/spf13/cobra | v1.9.1 | Apache-2.0 | Apache License 2.0 |
| github.com/spf13/pflag | v1.0.6 | BSD-3-Clause | BSD notice |
| github.com/xo/terminfo | v0.0.0-20220910002029-abceb7e1c41e | MIT | MIT notice |
| golang.org/x/sync | v0.20.0 | BSD-3-Clause | BSD notice |
| golang.org/x/sys | v0.45.0 | BSD-3-Clause | BSD notice |
| gopkg.in/yaml.v3 | v3.0.1 | MIT and Apache-2.0 | MIT notice, Apache License 2.0, upstream NOTICE |

## MIT Notices

The MIT-family modules listed above require preserving the copyright and
permission notices in substantial copies. Their upstream copyright notices are:

- charm.land/bubbles/v2: Copyright (c) 2020-2026 Charmbracelet, Inc.
- charm.land/bubbletea/v2: Copyright (c) 2020-2026 Charmbracelet, Inc.
- charm.land/lipgloss/v2: Copyright (c) 2021-2026 Charmbracelet, Inc.
- github.com/charmbracelet/colorprofile: Copyright (c) 2020-2024 Charmbracelet, Inc.
- github.com/charmbracelet/ultraviolet: Copyright (c) 2025 Charmbracelet, Inc.
- github.com/charmbracelet/x/ansi: Copyright (c) 2023 Charmbracelet, Inc.
- github.com/charmbracelet/x/term: Copyright (c) 2023 Charmbracelet, Inc.
- github.com/charmbracelet/x/termios: Copyright (c) 2023 Charmbracelet, Inc.
- github.com/charmbracelet/x/windows: Copyright (c) 2023 Charmbracelet, Inc.
- github.com/clipperhouse/displaywidth: Copyright (c) 2025 Matt Sherman.
- github.com/clipperhouse/uax29/v2: Copyright (c) 2020 Matt Sherman.
- github.com/lucasb-eyer/go-colorful: Copyright (c) 2013 Lucas Beyer.
- github.com/mattn/go-runewidth: Copyright (c) 2016 Yasuhiro Matsumoto.
- github.com/muesli/cancelreader: Copyright (c) 2022 Erik Geiser and Christian Muehlhaeuser.
- github.com/rivo/uniseg: Copyright (c) 2019 Oliver Kuederle.
- github.com/xo/terminfo: Copyright (c) 2016 Anmol Sethi.
- gopkg.in/yaml.v3 MIT-covered libyaml-derived files: Copyright (c) 2006-2010 Kirill Simonov; Copyright (c) 2006-2011 Kirill Simonov.

MIT License text:

```text
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## BSD-3-Clause Notices

The BSD-family modules listed above require preserving the copyright notice,
list of conditions, and disclaimer in binary distribution documentation or
other materials. Their upstream copyright notices are:

- github.com/atotto/clipboard: Copyright (c) 2013 Ato Araki. All rights reserved.
- github.com/spf13/pflag: Copyright (c) 2012 Alex Ogier. All rights reserved.; Copyright (c) 2012 The Go Authors. All rights reserved.
- golang.org/x/sync: Copyright 2009 The Go Authors.
- golang.org/x/sys: Copyright 2009 The Go Authors.

BSD-3-Clause license text:

```text
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.
* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.
* Neither the name of the copyright holder nor the names of its contributors
  may be used to endorse or promote products derived from this software without
  specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

Upstream non-endorsement clauses use the following specific names:

- github.com/atotto/clipboard: "Neither the name of @atotto. nor the names of
  its contributors may be used to endorse or promote products derived from this
  software without specific prior written permission."
- github.com/spf13/pflag: "Neither the name of Google Inc. nor the names of its
  contributors may be used to endorse or promote products derived from this
  software without specific prior written permission."
- golang.org/x/sync and golang.org/x/sys: "Neither the name of Google LLC nor
  the names of its contributors may be used to endorse or promote products
  derived from this software without specific prior written permission."

## Apache-2.0 Notices

The Apache-family modules listed above require preserving the Apache License
2.0 text and any upstream NOTICE file. Burpvalve's binary graph includes:

- github.com/spf13/cobra v1.9.1
- gopkg.in/yaml.v3 v3.0.1, for Apache-covered files

Apache License 2.0 text:

```text
Apache License
Version 2.0, January 2004
http://www.apache.org/licenses/

TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

1. Definitions.

"License" shall mean the terms and conditions for use, reproduction, and
distribution as defined by Sections 1 through 9 of this document.

"Licensor" shall mean the copyright owner or entity authorized by the copyright
owner that is granting the License.

"Legal Entity" shall mean the union of the acting entity and all other entities
that control, are controlled by, or are under common control with that entity.
For the purposes of this definition, "control" means (i) the power, direct or
indirect, to cause the direction or management of such entity, whether by
contract or otherwise, or (ii) ownership of fifty percent (50%) or more of the
outstanding shares, or (iii) beneficial ownership of such entity.

"You" (or "Your") shall mean an individual or Legal Entity exercising
permissions granted by this License.

"Source" form shall mean the preferred form for making modifications, including
but not limited to software source code, documentation source, and configuration
files.

"Object" form shall mean any form resulting from mechanical transformation or
translation of a Source form, including but not limited to compiled object code,
generated documentation, and conversions to other media types.

"Work" shall mean the work of authorship, whether in Source or Object form,
made available under the License, as indicated by a copyright notice that is
included in or attached to the work.

"Derivative Works" shall mean any work, whether in Source or Object form, that
is based on (or derived from) the Work and for which the editorial revisions,
annotations, elaborations, or other modifications represent, as a whole, an
original work of authorship. For the purposes of this License, Derivative Works
shall not include works that remain separable from, or merely link (or bind by
name) to the interfaces of, the Work and Derivative Works thereof.

"Contribution" shall mean any work of authorship, including the original version
of the Work and any modifications or additions to that Work or Derivative Works
thereof, that is intentionally submitted to Licensor for inclusion in the Work
by the copyright owner or by an individual or Legal Entity authorized to submit
on behalf of the copyright owner.

"Contributor" shall mean Licensor and any individual or Legal Entity on behalf
of whom a Contribution has been received by Licensor and subsequently
incorporated within the Work.

2. Grant of Copyright License. Subject to the terms and conditions of this
License, each Contributor hereby grants to You a perpetual, worldwide,
non-exclusive, no-charge, royalty-free, irrevocable copyright license to
reproduce, prepare Derivative Works of, publicly display, publicly perform,
sublicense, and distribute the Work and such Derivative Works in Source or
Object form.

3. Grant of Patent License. Subject to the terms and conditions of this License,
each Contributor hereby grants to You a perpetual, worldwide, non-exclusive,
no-charge, royalty-free, irrevocable patent license to make, have made, use,
offer to sell, sell, import, and otherwise transfer the Work.

4. Redistribution. You may reproduce and distribute copies of the Work or
Derivative Works thereof in any medium, with or without modifications, and in
Source or Object form, provided that You meet the following conditions:

(a) You must give any other recipients of the Work or Derivative Works a copy
of this License; and
(b) You must cause any modified files to carry prominent notices stating that
You changed the files; and
(c) You must retain, in the Source form of any Derivative Works that You
distribute, all copyright, patent, trademark, and attribution notices from the
Source form of the Work, excluding those notices that do not pertain to any part
of the Derivative Works; and
(d) If the Work includes a "NOTICE" text file as part of its distribution, then
any Derivative Works that You distribute must include a readable copy of the
attribution notices contained within such NOTICE file, excluding those notices
that do not pertain to any part of the Derivative Works, in at least one of the
following places: within a NOTICE text file distributed as part of the
Derivative Works; within the Source form or documentation, if provided along
with the Derivative Works; or within a display generated by the Derivative
Works, if and wherever such third-party notices normally appear.

You may add Your own copyright statement to Your modifications and may provide
additional or different license terms and conditions for use, reproduction, or
distribution of Your modifications, or for any such Derivative Works as a
whole, provided Your use, reproduction, and distribution of the Work otherwise
complies with the conditions stated in this License.

5. Submission of Contributions. Unless You explicitly state otherwise, any
Contribution intentionally submitted for inclusion in the Work by You to the
Licensor shall be under the terms and conditions of this License, without any
additional terms or conditions.

6. Trademarks. This License does not grant permission to use the trade names,
trademarks, service marks, or product names of the Licensor, except as required
for reasonable and customary use in describing the origin of the Work and
reproducing the content of the NOTICE file.

7. Disclaimer of Warranty. Unless required by applicable law or agreed to in
writing, Licensor provides the Work on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied, including, without
limitation, any warranties or conditions of TITLE, NON-INFRINGEMENT,
MERCHANTABILITY, or FITNESS FOR A PARTICULAR PURPOSE. You are solely
responsible for determining the appropriateness of using or redistributing the
Work and assume any risks associated with Your exercise of permissions under
this License.

8. Limitation of Liability. In no event and under no legal theory, whether in
tort (including negligence), contract, or otherwise, unless required by
applicable law or agreed to in writing, shall any Contributor be liable to You
for damages, including any direct, indirect, special, incidental, or
consequential damages of any character arising as a result of this License or
out of the use or inability to use the Work.

9. Accepting Warranty or Additional Liability. While redistributing the Work or
Derivative Works thereof, You may choose to offer, and charge a fee for,
acceptance of support, warranty, indemnity, or other liability obligations
and/or rights consistent with this License. However, in accepting such
obligations, You may act only on Your own behalf and on Your sole
responsibility, not on behalf of any other Contributor.
```

## gopkg.in/yaml.v3 Upstream NOTICE

The following upstream NOTICE text from `gopkg.in/yaml.v3` is preserved:

```text
Copyright 2011-2016 Canonical Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
