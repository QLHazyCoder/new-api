package model

import (
	"sort"
	"strings"
)

func GetOccupiedUserGroups() ([]string, error) {
	var rawGroups []string
	err := DB.Model(&User{}).
		Distinct(commonGroupCol).
		Where(commonGroupCol+" <> ?", "").
		Pluck(commonGroupCol, &rawGroups).Error
	if err != nil {
		return nil, err
	}

	groupSet := make(map[string]struct{}, len(rawGroups))
	for _, rawGroup := range rawGroups {
		group := strings.TrimSpace(rawGroup)
		if group != "" {
			groupSet[group] = struct{}{}
		}
	}
	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups, nil
}
