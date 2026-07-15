package common

import "errors"

func ResolveModelMapping(modelName string, mappingJSON string) (string, bool, error) {
	if mappingJSON == "" || mappingJSON == "{}" {
		return modelName, false, nil
	}

	modelMap := make(map[string]string)
	if err := Unmarshal([]byte(mappingJSON), &modelMap); err != nil {
		return modelName, false, errors.New("unmarshal_model_mapping_failed")
	}

	current := modelName
	visited := map[string]bool{current: true}
	mapped := false
	for {
		next, exists := modelMap[current]
		if !exists || next == "" {
			return current, mapped, nil
		}
		if next == current {
			return current, mapped, nil
		}
		if visited[next] {
			return modelName, false, errors.New("model_mapping_contains_cycle")
		}
		visited[next] = true
		current = next
		mapped = true
	}
}
