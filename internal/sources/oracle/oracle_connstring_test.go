package oracle

import "testing"

func TestBuildGoOraConnString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		user           string
		password       string
		connBase       string
		walletLocation string
		want           string
	}{
		{
			name:           "encodes credentials and wallet",
			user:           "user[client]",
			password:       "pa:ss@word",
			connBase:       "dbhost:1521/XEPDB1",
			walletLocation: "/tmp/my wallet",
			want:           "oracle://user%5Bclient%5D:pa%3Ass%40word@dbhost:1521/XEPDB1?ssl=true&wallet=%2Ftmp%2Fmy+wallet",
		},
		{
			name:           "no wallet",
			user:           "scott",
			password:       "tiger",
			connBase:       "dbhost:1521/ORCL",
			walletLocation: "",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL",
		},
		{
			name:           "does not double encode percent encoded user",
			user:           "app_user%5BCLIENT_A%5D",
			password:       "secret",
			connBase:       "dbhost:1521/ORCL",
			walletLocation: "",
			want:           "oracle://app_user%5BCLIENT_A%5D:secret@dbhost:1521/ORCL",
		},
		{
			name:           "uses trimmed wallet location",
			user:           "scott",
			password:       "tiger",
			connBase:       "dbhost:1521/ORCL",
			walletLocation: "  /tmp/wallet  ",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?ssl=true&wallet=%2Ftmp%2Fwallet",
		},
		{
			name:           "preserves existing query without wallet",
			user:           "scott",
			password:       "tiger",
			connBase:       "dbhost:1521/ORCL?transport_connect_timeout=30",
			walletLocation: "",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?transport_connect_timeout=30",
		},
		{
			name:           "merges existing query with wallet",
			user:           "scott",
			password:       "tiger",
			connBase:       "dbhost:1521/ORCL?foo=bar",
			walletLocation: "/tmp/wallet",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?foo=bar&ssl=true&wallet=%2Ftmp%2Fwallet",
		},
		{
			name:           "preserves malformed existing query when appending wallet",
			user:           "scott",
			password:       "tiger",
			connBase:       "dbhost:1521/ORCL?already=%ZZ",
			walletLocation: "/tmp/wallet",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?already=%ZZ&ssl=true&wallet=%2Ftmp%2Fwallet",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoOraConnString(tc.user, tc.password, tc.connBase, tc.walletLocation)
			if got != tc.want {
				t.Fatalf("buildGoOraConnString() = %q, want %q", got, tc.want)
			}
		})
	}
}
