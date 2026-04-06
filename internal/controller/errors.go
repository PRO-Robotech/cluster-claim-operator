/*
Copyright 2026.

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

package controller

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// TerminalError wraps an error to indicate it is non-recoverable.
// The pipeline sets phase=Failed and does not requeue for terminal errors.
type TerminalError struct {
	Err error
}

func (e *TerminalError) Error() string {
	return e.Err.Error()
}

func (e *TerminalError) Unwrap() error {
	return e.Err
}

// NewTerminalError wraps err as a TerminalError.
func NewTerminalError(err error) error {
	return &TerminalError{Err: err}
}

// NewTerminalErrorf creates a formatted TerminalError.
func NewTerminalErrorf(format string, args ...interface{}) error {
	return &TerminalError{Err: fmt.Errorf(format, args...)}
}

// IsTerminalError returns true if the error chain contains a TerminalError.
func IsTerminalError(err error) bool {
	var te *TerminalError
	return errors.As(err, &te)
}

// classifyAPIError wraps known non-recoverable K8s API errors as TerminalError.
// Conflict, timeout, and other transient errors are returned as-is.
func classifyAPIError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
		return NewTerminalError(err)
	}
	return err
}
