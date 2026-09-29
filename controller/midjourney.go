package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// midjourneyPollSummary is the result recorded on a midjourney_poll system task
// row, summarizing one polling pass.
type midjourneyPollSummary struct {
	UnfinishedTasks        int `json:"unfinished_tasks"`
	ChannelsScanned        int `json:"channels_scanned"`
	NullTasksPendingReview int `json:"null_tasks_pending_review"`
}

// runMidjourneyTaskUpdateOnce performs one Midjourney polling pass synchronously.
// It honors ctx cancellation (the system-task runner cancels it when the lease
// is lost) and, when report is non-nil, reports progress as (processedChannels,
// totalChannels) so the system task surfaces a percentage.
func runMidjourneyTaskUpdateOnce(ctx context.Context, report func(processed, total int)) midjourneyPollSummary {
	summary := midjourneyPollSummary{}
	if ctx == nil {
		ctx = context.Background()
	}

	tasks := model.GetAllUnFinishTasks()
	if len(tasks) == 0 {
		return summary
	}
	summary.UnfinishedTasks = len(tasks)

	logger.LogInfo(ctx, fmt.Sprintf("检测到未完成的任务数有: %v", len(tasks)))
	taskChannelM := make(map[int][]string)
	taskM := make(map[string]*model.Midjourney)
	nullTaskIds := make([]int, 0)
	for _, task := range tasks {
		if task.MjId == "" {
			// 统计失败的未完成任务
			nullTaskIds = append(nullTaskIds, task.Id)
			continue
		}
		taskM[task.MjId] = task
		taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], task.MjId)
	}
	if len(nullTaskIds) > 0 {
		for _, taskID := range nullTaskIds {
			changed, err := model.MarkMidjourneyTaskBillingReviewRequired(taskID)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Mark Midjourney task without upstream id for billing review failed: id=%d err=%v", taskID, err))
				continue
			}
			if changed {
				summary.NullTasksPendingReview++
				common.SysError(fmt.Sprintf("Midjourney task id=%d has no upstream task ID; quota is retained for manual billing review", taskID))
			}
		}
	}
	if len(taskChannelM) == 0 {
		return summary
	}

	totalChannels := len(taskChannelM)
	processedChannels := 0
	for channelId, taskIds := range taskChannelM {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(processedChannels, totalChannels)
		}
		processedChannels++
		summary.ChannelsScanned++
		logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
		if len(taskIds) == 0 {
			continue
		}
		midjourneyChannel, err := model.CacheGetChannel(channelId)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("CacheGetChannel: %v", err))
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		requestUrl := fmt.Sprintf("%s/mj/task/list-by-condition", *midjourneyChannel.BaseURL)

		body, err := common.Marshal(map[string]any{
			"ids": taskIds,
		})
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Get Task marshal body error: %v", err))
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		timeout := time.Second * 15
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(requestCtx, "POST", requestUrl, bytes.NewBuffer(body))
		if err != nil {
			cancel()
			logger.LogError(ctx, fmt.Sprintf("Get Task error: %v", err))
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("mj-api-secret", midjourneyChannel.Key)
		resp, err := service.GetHttpClient().Do(req)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Get Task Do req error: %v", err))
			cancel()
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
			resp.Body.Close()
			cancel()
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error: %v", err))
			resp.Body.Close()
			cancel()
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		var responseItems []dto.MidjourneyDto
		err = common.Unmarshal(responseBody, &responseItems)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error2: %v, body: %s", err, string(responseBody)))
			resp.Body.Close()
			cancel()
			markStaleMidjourneyTasksForBillingReview(ctx, taskChannelTasks(taskIds, taskM))
			continue
		}
		resp.Body.Close()
		req.Body.Close()
		cancel()

		for _, responseItem := range responseItems {
			task := taskM[responseItem.MjId]
			if task == nil {
				logger.LogWarn(ctx, fmt.Sprintf("Midjourney task response ignored: unknown mj_id=%s", responseItem.MjId))
				continue
			}

			terminalFailure := responseItem.Status == "FAILURE"
			terminalSuccess := responseItem.Status == "SUCCESS"
			terminal := terminalFailure || terminalSuccess
			useTime := (time.Now().UnixNano() / int64(time.Millisecond)) - task.SubmitTime
			if !terminal && (useTime > 3600000 || responseItem.Progress == "100%" || responseItem.FailReason != "") {
				changed, err := model.MarkMidjourneyTaskBillingReviewRequired(task.Id)
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("Mark uncertain Midjourney task for billing review failed: id=%d err=%v", task.Id, err))
				} else if changed {
					common.SysError(fmt.Sprintf("Midjourney task %s has no confirmed terminal upstream result; quota is retained and will not be refunded automatically", task.MjId))
				}
				task.BillingReviewPending = true
			}
			reviewPending := !terminal
			failureConfirmed := terminalFailure
			if !checkMjTaskNeedUpdate(task, responseItem) &&
				task.BillingReviewPending == reviewPending && task.BillingFailureConfirmed == failureConfirmed {
				continue
			}
			preStatus := task.Status
			task.Code = 1
			task.BillingReviewPending = reviewPending
			task.BillingFailureConfirmed = failureConfirmed
			task.Progress = responseItem.Progress
			task.PromptEn = responseItem.PromptEn
			task.State = responseItem.State
			task.SubmitTime = responseItem.SubmitTime
			task.StartTime = responseItem.StartTime
			task.FinishTime = responseItem.FinishTime
			task.ImageUrl = responseItem.ImageUrl
			task.Status = responseItem.Status
			task.FailReason = responseItem.FailReason
			if responseItem.Properties != nil {
				propertiesStr, _ := common.Marshal(responseItem.Properties)
				task.Properties = string(propertiesStr)
			}
			if responseItem.Buttons != nil {
				buttonStr, _ := common.Marshal(responseItem.Buttons)
				task.Buttons = string(buttonStr)
			}
			// 映射 VideoUrl
			task.VideoUrl = responseItem.VideoUrl

			// 映射 VideoUrls - 将数组序列化为 JSON 字符串
			if responseItem.VideoUrls != nil && len(responseItem.VideoUrls) > 0 {
				videoUrlsStr, err := common.Marshal(responseItem.VideoUrls)
				if err != nil {
					logger.LogError(ctx, fmt.Sprintf("序列化 VideoUrls 失败: %v", err))
					task.VideoUrls = "[]" // 失败时设置为空数组
				} else {
					task.VideoUrls = string(videoUrlsStr)
				}
			} else {
				task.VideoUrls = "" // 空值时清空字段
			}
			if terminal && task.FinishTime == 0 {
				task.FinishTime = time.Now().UnixMilli()
			}

			shouldReturnQuota := task.BillingFailureConfirmed
			if shouldReturnQuota {
				if task.FailReason == "" {
					task.FailReason = "上游报告任务失败"
				}
				logger.LogInfo(ctx, task.MjId+" 构建失败，"+task.FailReason)
				task.Progress = "100%"
			}
			won, err := task.UpdateWithStatus(preStatus)
			if err != nil {
				logger.LogError(ctx, "UpdateMidjourneyTask task error: "+err.Error())
			} else if won && shouldReturnQuota {
				service.RefundMidjourneyQuota(ctx, task, "构图失败")
			}
		}
	}
	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(totalChannels, totalChannels)
	}
	return summary
}

func taskChannelTasks(taskIDs []string, taskByID map[string]*model.Midjourney) []*model.Midjourney {
	tasks := make([]*model.Midjourney, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if task := taskByID[taskID]; task != nil {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func markStaleMidjourneyTasksForBillingReview(ctx context.Context, tasks []*model.Midjourney) {
	now := time.Now().UnixMilli()
	for _, task := range tasks {
		if task == nil || task.SubmitTime <= 0 || now-task.SubmitTime <= 3600000 {
			continue
		}
		changed, err := model.MarkMidjourneyTaskBillingReviewRequired(task.Id)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Mark stale Midjourney task for billing review failed: id=%d err=%v", task.Id, err))
		} else if changed {
			common.SysError(fmt.Sprintf("Midjourney task %s cannot be checked upstream after one hour; quota is retained and requires review", task.MjId))
		}
	}
}

func checkMjTaskNeedUpdate(oldTask *model.Midjourney, newTask dto.MidjourneyDto) bool {
	if oldTask.Code != 1 {
		return true
	}
	if oldTask.Progress != newTask.Progress {
		return true
	}
	if oldTask.PromptEn != newTask.PromptEn {
		return true
	}
	if oldTask.State != newTask.State {
		return true
	}
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.ImageUrl != newTask.ImageUrl {
		return true
	}
	if oldTask.Status != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if oldTask.BillingReviewPending && (newTask.Status == "SUCCESS" || newTask.Status == "FAILURE") {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.Progress != "100%" && newTask.FailReason != "" {
		return true
	}
	// 检查 VideoUrl 是否需要更新
	if oldTask.VideoUrl != newTask.VideoUrl {
		return true
	}
	// 检查 VideoUrls 是否需要更新
	if newTask.VideoUrls != nil && len(newTask.VideoUrls) > 0 {
		newVideoUrlsStr, _ := common.Marshal(newTask.VideoUrls)
		if oldTask.VideoUrls != string(newVideoUrlsStr) {
			return true
		}
	} else if oldTask.VideoUrls != "" {
		// 如果新数据没有 VideoUrls 但旧数据有，需要更新（清空）
		return true
	}

	return false
}

func GetAllMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	// 解析其他查询参数
	queryParams := model.TaskQueryParams{
		ChannelID:      c.Query("channel_id"),
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllTasks(queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetUserMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	queryParams := model.TaskQueryParams{
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllUserTask(userId, queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
