package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

func HashPassword(password string) (string, error) {
	if len([]rune(password)) < 16 {
		return "", errors.New("密码至少需要 16 个字符")
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	parallelism := uint8(argonParallelism)
	if runtime.NumCPU() == 1 {
		parallelism = 1
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, parallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonIterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	params, salt, expected, err := parseArgon2ID(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

type argonParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parseArgon2ID(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(strings.TrimSpace(encoded), "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, errors.New("密码哈希格式无效")
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return argonParams{}, nil, nil, errors.New("不支持的 Argon2 版本")
	}
	var params argonParams
	for _, entry := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(entry, "=", 2)
		if len(keyValue) != 2 {
			return argonParams{}, nil, nil, errors.New("Argon2 参数无效")
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil {
			return argonParams{}, nil, nil, errors.New("Argon2 参数无效")
		}
		switch keyValue[0] {
		case "m":
			params.memory = uint32(value)
		case "t":
			params.iterations = uint32(value)
		case "p":
			params.parallelism = uint8(value)
		}
	}
	if params.memory == 0 || params.iterations == 0 || params.parallelism == 0 {
		return argonParams{}, nil, nil, errors.New("Argon2 参数缺失")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, errors.New("Argon2 盐值无效")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, errors.New("Argon2 摘要无效")
	}
	return params, salt, hash, nil
}

func RunPasswordHasher(input io.Reader, output io.Writer) error {
	inputFile, ok := input.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(inputFile.Fd())) {
		return errors.New("hash-password 必须在交互式终端中运行")
	}
	fmt.Fprint(os.Stderr, "请输入访问密码（至少 16 个字符）：")
	first, err := term.ReadPassword(int(inputFile.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "请再次输入访问密码：")
	second, err := term.ReadPassword(int(inputFile.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(first, second) != 1 {
		return errors.New("两次输入的密码不一致")
	}
	hash, err := HashPassword(string(first))
	if err != nil {
		return err
	}
	fmt.Fprintln(output, hash)
	return nil
}
