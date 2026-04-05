package envfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	publicFile  = "http-client.env.json"
	privateFile = "http-client.private.env.json"
)

type Loaded struct {
	Public map[string]string
	Secret map[string]string
}

func LoadForRequestFile(requestPath, envName string) (Loaded, error) {
	if envName == "" {
		return Loaded{
			Public: map[string]string{},
			Secret: map[string]string{},
		}, nil
	}

	dir := filepath.Dir(requestPath)
	publicVars, publicExists, err := loadOne(filepath.Join(dir, publicFile), envName)
	if err != nil {
		return Loaded{}, err
	}
	secretVars, secretExists, err := loadOne(filepath.Join(dir, privateFile), envName)
	if err != nil {
		return Loaded{}, err
	}

	if !publicExists && !secretExists {
		return Loaded{}, fmt.Errorf("environment %q not found in %s or %s", envName, publicFile, privateFile)
	}

	return Loaded{
		Public: publicVars,
		Secret: secretVars,
	}, nil
}

func loadOne(path, envName string) (map[string]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, err
	}

	var root map[string]map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	env, ok := root[envName]
	if !ok {
		return map[string]string{}, false, nil
	}

	flattened := make(map[string]string, len(env))
	for key, value := range env {
		flattened[key] = stringify(value)
	}

	return flattened, true, nil
}

func stringify(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}
