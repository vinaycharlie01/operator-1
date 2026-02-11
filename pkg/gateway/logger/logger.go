// Copyright (C) 2024, MinIO, Inc.
//
// This code is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License, version 3,
// as published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License, version 3,
// along with this program.  If not, see <http://www.gnu.org/licenses/>

package logger

import (
	"log"
)

// Logger implements the Envoy xDS SnapshotCache logger interface
type Logger struct {
	Debug bool
}

// Debugf logs debug messages
func (l Logger) Debugf(format string, args ...interface{}) {
	if l.Debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Infof logs info messages
func (l Logger) Infof(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// Warnf logs warning messages
func (l Logger) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

// Errorf logs error messages
func (l Logger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// Made with Bob
