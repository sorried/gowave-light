package users

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenerateShortUUID(t *testing.T) {
	for _, length := range []int{-1, 0, 1, 16, 32, 64, 1024} {
		want := length
		if want <= 0 {
			want = 16
		}
		value := GenerateShortUUID(length)
		if len(value) != want {
			t.Fatalf("length %d: got %d characters, want %d", length, len(value), want)
		}
		for _, char := range value {
			if !strings.ContainsRune(alphabet, char) {
				t.Fatalf("unexpected character %q", char)
			}
		}
	}
}

func BenchmarkGenerateShortUUID(b *testing.B) {
	for _, length := range []int{16, 32, 64} {
		b.Run(fmt.Sprint(length), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if len(GenerateShortUUID(length)) != length {
					b.Fatal("unexpected identifier length")
				}
			}
		})
	}
}
