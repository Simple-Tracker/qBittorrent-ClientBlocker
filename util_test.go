package main

import (
	"sync"
	"testing"
)

func TestEraseSyncMap(t *testing.T) {
	testCases := []struct {
		name string
		data map[any]any
	}{
		{
			name: "TestEraseSyncMap",
			data: map[any]any{"key": "val"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &sync.Map{}
			for k, v := range tc.data {
				m.Store(k, v)
			}

			EraseSyncMap(m)
			m.Range(func(k, v any) bool {
				t.Errorf("EraseSyncMap() = %v, want %v", v, nil)
				return false
			})
		})
	}
}
