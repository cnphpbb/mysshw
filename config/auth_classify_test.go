package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyAuth(t *testing.T) {
	tests := []struct {
		name     string
		password string
		keyPath  string
		cfgAlgo  string // "" / "bcrypt" / "bcrypt-pepper"
		want     string
	}{
		{"nil-password + key", "", "/root/.ssh/id_rsa", "", "key"},
		{"nil-password no key", "", "", "", "empty"},
		{"bcrypt hash no pepper", "$2a$10$BBEDysNBBCnf/RwKMLiea.ufVrsybvoes.HYz8w.6wtff7TSLhjsC", "", "", "bcrypt"},
		{"bcrypt hash + pepper algo", "$2a$10$BBEDysNBBCnf/RwKMLiea.ufVrsybvoes.HYz8w.6wtff7TSLhjsC", "", "bcrypt-pepper", "bcrypt-pepper"},
		{"plaintext password", "my-secret", "", "", "password"},
		{"env var", "${MY_PWD}", "", "", "env"},
		{"plaintext + key", "my-secret", "/key", "", "password+key"},
		{"env + key", "${MY_PWD}", "/key", "", "env+key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &SSHNode{Password: tt.password, KeyPath: tt.keyPath}
			var cfg *Configs
			if tt.cfgAlgo != "" {
				cfg = &Configs{EncryptionAlgorithm: tt.cfgAlgo}
			}
			assert.Equal(t, tt.want, ClassifyAuth(n, cfg))
		})
	}
}

func TestClassifyAuth_NilNode(t *testing.T) {
	assert.Equal(t, "unknown", ClassifyAuth(nil, nil))
}
