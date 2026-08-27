package net

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchWebsocketFunc(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		data  string
		match bool
	}{
		{name: "no path, default path", data: "GET " + FrpWebsocketPath + " HTTP/1.1", match: true},
		{name: "no path, custom path", data: "GET /custom HTTP/1.1", match: true},
		{name: "no path, root path", data: "GET / HTTP/1.1", match: true},
		{name: "no path, not a GET request", data: "POST /custom HTTP/1.1", match: false},
		{name: "no path, not http", data: "\x17\x03\x03\x00", match: false},
		{
			name:  "with paths, matched",
			paths: []string{FrpWebsocketPath, "/frp"},
			data:  "GET /frp HTTP/1.1",
			match: true,
		},
		{
			name:  "with paths, matched with query string",
			paths: []string{"/frp"},
			data:  "GET /frp?a=b HTTP/1.1",
			match: true,
		},
		{
			name:  "with paths, unmatched path",
			paths: []string{"/frp"},
			data:  "GET /other HTTP/1.1",
			match: false,
		},
		{
			name:  "with paths, path is only a prefix of the request target",
			paths: []string{"/frp"},
			data:  "GET /frpx HTTP/1.1",
			match: false,
		},
		{
			name:  "with paths, default path not accepted",
			paths: []string{"/frp"},
			data:  "GET " + FrpWebsocketPath + " HTTP/1.1",
			match: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require := require.New(t)
			needBytesNum, matchFn := MatchWebsocketFunc(test.paths)
			require.LessOrEqual(int(needBytesNum), len(test.data))
			// The mux only hands over the number of bytes the listener asked for.
			require.Equal(test.match, matchFn([]byte(test.data[:needBytesNum])))
		})
	}
}
