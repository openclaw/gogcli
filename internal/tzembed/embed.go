// Package tzembed forces embedding of the IANA timezone database
// so time.LoadLocation works on Windows in both binaries and tests.
package tzembed

import _ "time/tzdata" // Embeds IANA timezone database so time.LoadLocation works on Windows
