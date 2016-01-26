package util

import (
	"fmt"
	"strings"

	"code.google.com/p/go-uuid/uuid"
)

func ExtractUUIDs(names []string, prefix, suffix string) ([]string, error) {
	result := []string{}
	for i := range names {
		f := names[i]
		// Remove additional slash if exists
		f = strings.TrimLeft(f, "/")
		f = strings.TrimPrefix(f, prefix)
		f = strings.TrimSuffix(f, suffix)
		if !ValidateUUID(f) {
			return nil, fmt.Errorf("Invalid name %v was processed to extract UUID with prefix %v surfix %v", names[i], prefix, suffix)
		}
		result = append(result, f)
	}
	return result, nil
}

func ValidateUUID(s string) bool {
	return uuid.Parse(s) != nil
}

func CheckUUID(uuid string) error {
	if !ValidateUUID(uuid) {
		return fmt.Errorf("Invalid UUID %v", uuid)
	}
	return nil
}

func GetUUID(v interface{}, key string, required bool, err error) (string, error) {
	uuid, err := GetFlag(v, key, required, err)
	if err != nil {
		return uuid, err
	}
	uuid = strings.ToLower(uuid)
	if !required && uuid == "" {
		return "", nil
	}
	if err := CheckUUID(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}
