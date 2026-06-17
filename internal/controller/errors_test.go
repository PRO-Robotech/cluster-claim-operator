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
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsTerminalError(t *testing.T) {
	base := fmt.Errorf("something broke")
	terminal := NewTerminalError(base)

	if !IsTerminalError(terminal) {
		t.Error("expected IsTerminalError to return true for TerminalError")
	}
	if !errors.Is(terminal, base) {
		t.Error("expected errors.Is to find base error through TerminalError")
	}
}

func TestIsTerminalError_Wrapped(t *testing.T) {
	terminal := NewTerminalError(fmt.Errorf("template not found"))
	wrapped := fmt.Errorf("step Application: %w", terminal)

	if !IsTerminalError(wrapped) {
		t.Error("expected IsTerminalError to return true for wrapped TerminalError")
	}
}

func TestIsTerminalError_PlainError(t *testing.T) {
	if IsTerminalError(fmt.Errorf("just a normal error")) {
		t.Error("expected IsTerminalError to return false for plain error")
	}
}

func TestIsTerminalError_Nil(t *testing.T) {
	if IsTerminalError(nil) {
		t.Error("expected IsTerminalError to return false for nil")
	}
}

func TestNewTerminalErrorf(t *testing.T) {
	err := NewTerminalErrorf("template %q not found", "my-template")
	if !IsTerminalError(err) {
		t.Error("expected IsTerminalError to return true")
	}
	expected := `template "my-template" not found`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestClassifyAPIError_Nil(t *testing.T) {
	if classifyAPIError(nil) != nil {
		t.Error("nil should remain nil")
	}
}

func TestClassifyAPIError_Conflict(t *testing.T) {
	conflict := apierrors.NewConflict(schema.GroupResource{}, "test", fmt.Errorf("modified"))
	if IsTerminalError(classifyAPIError(conflict)) {
		t.Error("conflict should be transient")
	}
}

func TestClassifyAPIError_Invalid(t *testing.T) {
	invalid := apierrors.NewInvalid(schema.GroupKind{}, "test", nil)
	if !IsTerminalError(classifyAPIError(invalid)) {
		t.Error("invalid should be terminal")
	}
}

func TestClassifyAPIError_Forbidden(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{}, "test", fmt.Errorf("denied"))
	if !IsTerminalError(classifyAPIError(forbidden)) {
		t.Error("forbidden should be terminal")
	}
}

func TestClassifyAPIError_WrappedInvalid(t *testing.T) {
	invalid := apierrors.NewInvalid(schema.GroupKind{}, "test", nil)
	wrapped := fmt.Errorf("create resource ns/name: %w", invalid)
	if !IsTerminalError(classifyAPIError(wrapped)) {
		t.Error("wrapped invalid should be terminal")
	}
}
