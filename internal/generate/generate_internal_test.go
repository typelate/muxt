package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_goGenerateEnv(t *testing.T) {
	t.Run("simple main package", func(t *testing.T) {
		p, f, n, err := goGenerateEnv(func(name string) (string, bool) {
			switch name {
			case goPackageEnvVar:
				return "pack", true
			case goFileEnvVar:
				return "file", true
			case goLineEnvVar:
				return "1234", true
			default:
				return "", false
			}
		})
		assert.NoError(t, err)
		assert.Equal(t, "pack", p)
		assert.Equal(t, "file", f)
		assert.Equal(t, 1234, n)
	})
	t.Run("package", func(t *testing.T) {
		_, _, _, err := goGenerateEnv(func(name string) (string, bool) {
			switch name {
			//case goPackageEnvVar:
			//	return "pack", true
			case goFileEnvVar:
				return "file", true
			case goLineEnvVar:
				return "1234", true
			default:
				return "", false
			}
		})
		assert.ErrorContains(t, err, goPackageEnvVar)
	})
	t.Run("file", func(t *testing.T) {
		_, _, _, err := goGenerateEnv(func(name string) (string, bool) {
			switch name {
			case goPackageEnvVar:
				return "pack", true
			//case goFileEnvVar:
			//	return "file", true
			case goLineEnvVar:
				return "1234", true
			default:
				return "", false
			}
		})
		assert.ErrorContains(t, err, goFileEnvVar)
	})
	t.Run("line", func(t *testing.T) {
		_, _, _, err := goGenerateEnv(func(name string) (string, bool) {
			switch name {
			case goPackageEnvVar:
				return "pack", true
			case goFileEnvVar:
				return "file", true
			//case goLineEnvVar:
			//	return "1234", true
			default:
				return "", false
			}
		})
		assert.ErrorContains(t, err, goLineEnvVar)
	})
	t.Run("bad number", func(t *testing.T) {
		_, _, _, err := goGenerateEnv(func(name string) (string, bool) {
			switch name {
			case goPackageEnvVar:
				return "pack", true
			case goFileEnvVar:
				return "file", true
			case goLineEnvVar:
				return "NUMBER", true
			default:
				return "", false
			}
		})
		assert.ErrorContains(t, err, "invalid syntax")
	})
}
