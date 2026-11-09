package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/bcrypt"
)

const legacyPassword = "ENC(zsvHz8TN9c/S2sXY3s/Y3s/Z3vXM6piamJmL)"

func TestPasswordFormats(t *testing.T) {
	for _, password := range []string{"", "test_f@2023!", "ENC(AAAA)", legacyPassword, "ENC(unfinished", "测试密码 ' \" \\ $ !"} {
		encoded := EncryptPassword(password)
		decoded, err := DecryptPassword(encoded)
		if err != nil || decoded != password {
			t.Fatalf("round trip failed: decoded=%q, err=%v", decoded, err)
		}
	}
	if got, err := DecryptPassword(legacyPassword); err != nil || got != "test_f@2023!" {
		t.Fatalf("legacy password failed: decoded=%q, err=%v", got, err)
	}
	for _, malformed := range []string{
		"ENC()", "ENC(AAAA)", "ENC(invalid!)", "ENC(unfinished",
		"ENC(AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA)",
	} {
		if _, err := DecryptPassword(malformed); err == nil {
			t.Fatalf("expected format error for %q", malformed)
		} else if strings.Contains(err.Error(), malformed) {
			t.Fatal("format error exposes the password")
		}
	}
	for _, tt := range []struct {
		mode, input, want string
		wantError         bool
	}{
		{"", legacyPassword, "test_f@2023!", false},
		{"auto", "plain-password", "plain-password", false},
		{"", "ENC(unfinished!", "ENC(unfinished!", false},
		{"auto", "ENC(unfinished!", "ENC(unfinished!", false},
		{"auto", "ENC(", "ENC(", false},
		{"auto", "plain-password)", "plain-password)", false},
		{"plain", "ENC(AAAA)", "ENC(AAAA)", false},
		{"plain", legacyPassword, legacyPassword, false},
		{"encrypted", legacyPassword, "test_f@2023!", false},
		{"encrypted", "plain-password", "", true},
		{"encrypted", "ENC(unfinished!", "", true},
		{"encrypted", "ENC(AAAA)", "", true},
		{"auto", "ENC(AAAA)", "", true},
		{"unknown", "plain-password", "", true},
	} {
		got, err := DecodePassword(tt.input, tt.mode)
		if (err != nil) != tt.wantError || got != tt.want {
			t.Fatalf("mode %q: decoded=%q, err=%v", tt.mode, got, err)
		}
	}
}

func TestConfigPasswordLifecycle(t *testing.T) {
	for _, tt := range []struct{ name, input, mode, want string }{
		{"ordinary", "test_f@2023!", "", "test_f@2023!"},
		{"incomplete marker", "ENC(unfinished!", "auto", "ENC(unfinished!"},
		{"legacy encrypted", legacyPassword, "", "test_f@2023!"},
		{"explicit encrypted", legacyPassword, "encrypted", "test_f@2023!"},
		{"nested encrypted", EncryptPassword("ENC(AAAA)"), "auto", "ENC(AAAA)"},
		{"literal marker", "ENC(AAAA)", "plain", "ENC(AAAA)"},
		{"literal valid ciphertext", legacyPassword, "plain", legacyPassword},
		{"quotes and unicode", "测试密码 ' \" \\ $ !", "plain", "测试密码 ' \" \\ $ !"},
	} {
		for _, encrypt := range []bool{false, true} {
			name := tt.name + "/plain storage"
			if encrypt {
				name = tt.name + "/encrypted storage"
			}
			t.Run(name, func(t *testing.T) {
				cfg := DefaultMultiSourceConfig
				cfg.EncodeConfigPwd = encrypt
				cfg.EnableBasicAuth, cfg.BasicAuthUsername, cfg.BasicAuthPassword = true, "monitor", legacyPassword
				ds := DefaultDataSourceConfig
				ds.Name, ds.DbHost, ds.DbUser = "test", "127.0.0.1:5236", "monitor"
				ds.DbPwd, ds.DbPwdMode = tt.input, tt.mode
				cfg.DataSources = []DataSourceConfig{ds}
				path := filepath.Join(t.TempDir(), "config.toml")
				if err := SaveMultiSourceConfig(&cfg, path); err != nil {
					t.Fatal(err)
				}
				loaded, err := LoadMultiSourceConfig(path)
				if err != nil {
					t.Fatal(err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					loaded, err = CheckAndEncryptConfigPasswords(loaded, path)
					if err != nil {
						t.Fatal(err)
					}
					if encrypt {
						var stored rawMultiSourceConfig
						if _, err := toml.DecodeFile(path, &stored); err != nil {
							t.Fatal(err)
						}
						password, err := DecodePassword(stored.DataSources[0].DbPwd, "encrypted")
						if err != nil || password != tt.want || stored.DataSources[0].DbPwdMode == "plain" {
							t.Fatalf("password was not stored as valid ciphertext: %v", err)
						}
						if stored.BasicAuthPassword != legacyPassword {
							t.Fatal("database password encryption rewrote the basic auth password")
						}
					}
					if err := loaded.DecryptPasswords(); err != nil {
						t.Fatal(err)
					}
					if err := MergeMultiSourceConfigFromCmdArgs(loaded, &CmdArgs{}); err != nil {
						t.Fatal(err)
					}
					if loaded.DataSources[0].DbPwd != tt.want {
						t.Fatal("configuration changed the database password")
					}
					if loaded.BasicAuthPassword != "test_f@2023!" {
						t.Fatal("database password handling changed the basic auth password")
					}
					loaded, err = LoadMultiSourceConfig(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				// Saving a decoded configuration must not interpret a literal ENC(...) again.
				if err := SaveMultiSourceConfig(loaded, path); err != nil {
					t.Fatal(err)
				}
				loaded, err = LoadMultiSourceConfig(path)
				if err != nil || loaded.DataSources[0].DbPwd != tt.want {
					t.Fatalf("decoded configuration reload failed: %v", err)
				}
			})
		}
	}
}

func TestMalformedConfigPasswordReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[[datasource]]\nname='test'\ndbHost='127.0.0.1:5236'\ndbUser='monitor'\ndbPwd='ENC(AAAA)'\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMultiSourceConfig(path); err == nil || !strings.Contains(err.Error(), "dbPwdMode=plain") {
		t.Fatalf("expected actionable format error, got %v", err)
	}
}

func TestCommandLinePasswordDecoding(t *testing.T) {
	for _, tt := range []struct {
		name, input, mode, want string
		wantError               bool
	}{
		{"encrypted", legacyPassword, "auto", "test_f@2023!", false},
		{"incomplete marker", "ENC(unfinished!", "auto", "ENC(unfinished!", false},
		{"literal marker", "ENC(AAAA)", "plain", "ENC(AAAA)", false},
		{"literal valid ciphertext", legacyPassword, "plain", legacyPassword, false},
		{"nested encrypted", EncryptPassword("ENC(AAAA)"), "auto", "ENC(AAAA)", false},
		{"malformed", "ENC(AAAA)", "auto", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := passwordTestCmdArgs()
			*args.DbPwd, *args.DbPwdMode = tt.input, tt.mode
			cfg := DefaultMultiSourceConfig
			err := MergeMultiSourceConfigFromCmdArgs(&cfg, args)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected merge error: %v", err)
			}
			if err != nil {
				return
			}
			if err := cfg.DecryptPasswords(); err != nil {
				t.Fatal(err)
			}
			if cfg.DataSources[0].DbPwd != tt.want {
				t.Fatal("command line password was not decoded exactly once")
			}
		})
	}
}

func TestCommandLinePreservesExistingNumericParameters(t *testing.T) {
	args := passwordTestCmdArgs()
	*args.DbPwd = "test-password"
	*args.QueryTimeout, *args.MaxOpenConns = 301, 101
	cfg := DefaultMultiSourceConfig
	if err := MergeMultiSourceConfigFromCmdArgs(&cfg, args); err != nil {
		t.Fatalf("password handling changed unrelated command line validation: %v", err)
	}
	if cfg.DataSources[0].QueryTimeout != 301 || cfg.DataSources[0].MaxOpenConns != 101 {
		t.Fatal("command line numeric parameters were changed")
	}
}

func TestBasicAuthPasswordCompatibility(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("auth-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, input, want string
		wantError         bool
	}{
		{"plain", "auth-password", "auth-password", false},
		{"incomplete marker", "ENC(unfinished!", "ENC(unfinished!", false},
		{"bcrypt", string(hash), string(hash), false},
		{"legacy encrypted", legacyPassword, "test_f@2023!", false},
		{"malformed encrypted", "ENC(AAAA)", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultMultiSourceConfig
			cfg.EnableBasicAuth, cfg.BasicAuthUsername, cfg.BasicAuthPassword = true, "monitor", tt.input
			ds := DefaultDataSourceConfig
			ds.Name, ds.DbHost, ds.DbUser, ds.DbPwd = "test", "127.0.0.1:5236", "monitor", "test-password"
			cfg.DataSources = []DataSourceConfig{ds}
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := SaveMultiSourceConfig(&cfg, path); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadMultiSourceConfig(path)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected basic auth config error: %v", err)
			}
			if err == nil && loaded.BasicAuthPassword != tt.want {
				t.Fatal("configuration changed the basic auth password")
			}

			// CLI Basic Auth passwords retain their previous raw-input semantics.
			args := passwordTestCmdArgs()
			*args.DbPwd, *args.EnableBasicAuth = "test-password", true
			*args.BasicAuthPassword = tt.input
			if err := MergeMultiSourceConfigFromCmdArgs(&cfg, args); err != nil {
				t.Fatal(err)
			}
			if cfg.BasicAuthPassword != tt.input {
				t.Fatal("command line changed the basic auth password")
			}
		})
	}
}

func passwordTestCmdArgs() *CmdArgs {
	args := &CmdArgs{}
	// Allocate flag values as the command line parser does, without sharing its global state.
	fields := reflect.ValueOf(args).Elem()
	for i := 0; i < fields.NumField(); i++ {
		fields.Field(i).Set(reflect.New(fields.Field(i).Type().Elem()))
	}
	*args.DbHost, *args.DbUser = "127.0.0.1:5236", "monitor"
	*args.QueryTimeout, *args.MaxOpenConns = 5, 10
	*args.ListenAddr, *args.MetricPath = ":9200", "/metrics"
	return args
}
