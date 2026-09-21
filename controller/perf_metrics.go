package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func perfMetricsAllowedGroups(c *gin.Context) []string {
	allGroups := ratio_setting.GetGroupRatioCopy()
	allowed := make(map[string]struct{}, len(allGroups)+1)
	visibleGroups := service.GetUserUsableGroups("")
	if rawID, ok := c.Get("id"); ok {
		if userID, ok := rawID.(int); ok && userID > 0 {
			if user, err := model.GetUserCache(userID); err == nil {
				if user.Role >= common.RoleAdminUser {
					for group := range allGroups {
						allowed[group] = struct{}{}
					}
				} else {
					visibleGroups = service.GetUserUsableGroups(user.Group)
				}
			}
		}
	}
	if len(allowed) == 0 {
		for group := range visibleGroups {
			allowed[group] = struct{}{}
		}
	}
	groups := make([]string, 0, len(allowed))
	for group := range allGroups {
		// auto is a virtual request group and is visible to every user.
		if group == "auto" {
			continue
		}
		if _, ok := allowed[group]; ok {
			groups = append(groups, group)
		}
	}
	groups = append(groups, "auto")
	return groups
}

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := perfMetricsAllowedGroups(c)
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

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model:         modelName,
		Group:         c.Query("group"),
		Hours:         hours,
		AllowedGroups: perfMetricsAllowedGroups(c),
	})
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
