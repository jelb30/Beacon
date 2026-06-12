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

package observability

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInitTracingWithWriter(t *testing.T) {
	var output bytes.Buffer
	shutdown, err := InitTracingWithWriter(context.Background(), &output)
	if err != nil {
		t.Fatalf("InitTracingWithWriter returned error: %v", err)
	}

	ctx, span := Tracer("test").Start(context.Background(), "test-span")
	span.End()

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
	if !strings.Contains(output.String(), "test-span") {
		t.Fatalf("trace output did not contain span name: %s", output.String())
	}
}
