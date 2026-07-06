package controller

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := visiblePerfMetricGroups(c)
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	visibleGroups := visiblePerfMetricGroups(c)
	group := c.Query("group")
	if group != "" && !groupAllowed(group, visibleGroups) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": perfmetrics.QueryResult{
				ModelName: modelName,
				Groups:    []perfmetrics.GroupResult{},
			},
		})
		return
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: group,
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterVisibleGroups(result.Groups, visibleGroups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func activePerfMetricGroups() []string {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	groupSet := make(map[string]struct{}, len(activeRatios)+1)
	for group := range activeRatios {
		groupSet[group] = struct{}{}
	}
	groupSet["auto"] = struct{}{}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups
}

func visiblePerfMetricGroups(c *gin.Context) []string {
	activeGroups := activePerfMetricGroups()
	if len(activeGroups) == 0 {
		return []string{}
	}

	user, ok := currentPerfMetricUser(c)
	if ok && user.Role >= common.RoleAdminUser {
		return activeGroups
	}

	usableGroups := service.GetUserUsableGroups("")
	if ok {
		usableGroups = service.GetUserUsableGroups(user.Group)
	}
	return intersectGroups(activeGroups, usableGroups)
}

func currentPerfMetricUser(c *gin.Context) (*model.User, bool) {
	userId := contextUserId(c)
	if userId <= 0 {
		return nil, false
	}
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil || user.Status != common.UserStatusEnabled {
		return nil, false
	}
	return user, true
}

func contextUserId(c *gin.Context) int {
	raw, exists := c.Get("id")
	if !exists {
		return 0
	}
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func intersectGroups(activeGroups []string, usableGroups map[string]string) []string {
	if len(activeGroups) == 0 || len(usableGroups) == 0 {
		return []string{}
	}
	groups := make([]string, 0, len(activeGroups))
	for _, group := range activeGroups {
		if _, ok := usableGroups[group]; ok {
			groups = append(groups, group)
		}
	}
	return groups
}

func groupAllowed(group string, visibleGroups []string) bool {
	for _, visibleGroup := range visibleGroups {
		if group == visibleGroup {
			return true
		}
	}
	return false
}

func filterVisibleGroups(groups []perfmetrics.GroupResult, visibleGroups []string) []perfmetrics.GroupResult {
	if len(groups) == 0 || len(visibleGroups) == 0 {
		return []perfmetrics.GroupResult{}
	}
	visibleSet := make(map[string]struct{}, len(visibleGroups))
	for _, group := range visibleGroups {
		visibleSet[group] = struct{}{}
	}
	filtered := make([]perfmetrics.GroupResult, 0, len(groups))
	for _, group := range groups {
		if _, ok := visibleSet[group.Group]; ok {
			filtered = append(filtered, group)
		}
	}
	return filtered
}
