// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import "testing"

func TestReplaceDatabaseInDSN(t *testing.T) {
	t.Parallel()

	got, err := replaceDatabaseInDSN("postgres://app:app_pw@127.0.0.1:5432/app_db?sslmode=disable&application_name=bv", "bv_e2e_123")
	if err != nil {
		t.Fatalf("replaceDatabaseInDSN: %v", err)
	}
	want := "postgres://app:app_pw@127.0.0.1:5432/bv_e2e_123?sslmode=disable&application_name=bv"
	if got != want {
		t.Fatalf("replaceDatabaseInDSN() = %q, want %q", got, want)
	}
}
