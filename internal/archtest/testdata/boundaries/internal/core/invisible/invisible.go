// SPDX-License-Identifier: GPL-2.0-only

// Package invisible breaks rule 10 on three lines.
package invisible

// Name hides a right-to-left override: "user‮txt.exe", twice on this line: ‮.
const Name = "ro​ot"

// Escape has a raw escape character.
const Escape = "[31m"

// Fine writes them as escapes, and may hold any visible character: "…", "é", "李".
const Fine = "\u202E\u200B\x1b"
