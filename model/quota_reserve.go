package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type cacheQuotaResult int

const (
	cacheQuotaInsufficient cacheQuotaResult = iota
	cacheQuotaOK
	cacheQuotaMiss
)

const userQuotaReserveScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
local function normalize(value)
  value = value or '0'
  local sign = ''
  local first = string.sub(value, 1, 1)
  if first == '-' then
    sign = '-'
    value = string.sub(value, 2)
  elseif first == '+' then
    value = string.sub(value, 2)
  end
  value = string.gsub(value, '^0+', '')
  if value == '' then value = '0' end
  if value == '0' then return '0' end
  return sign .. value
end
local function compare(a, b)
  a = normalize(a)
  b = normalize(b)
  local aNegative = string.sub(a, 1, 1) == '-'
  local bNegative = string.sub(b, 1, 1) == '-'
  if aNegative and not bNegative then return -1 end
  if not aNegative and bNegative then return 1 end
  local aa = aNegative and string.sub(a, 2) or a
  local bb = bNegative and string.sub(b, 2) or b
  if string.len(aa) < string.len(bb) then return aNegative and 1 or -1 end
  if string.len(aa) > string.len(bb) then return aNegative and -1 or 1 end
  if aa == bb then return 0 end
  if aa < bb then return aNegative and 1 or -1 end
  return aNegative and -1 or 1
end
local quota = redis.call('HGET', KEYS[1], 'Quota')
if quota == false or compare(quota, ARGV[1]) < 0 then
  return 0
end
redis.call('HINCRBY', KEYS[1], 'Quota', '-' .. ARGV[1])
return 1`

const userQuotaDeltaScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'Quota', ARGV[1])
return 1`

const tokenQuotaReserveScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
local function normalize(value)
  value = value or '0'
  local sign = ''
  local first = string.sub(value, 1, 1)
  if first == '-' then
    sign = '-'
    value = string.sub(value, 2)
  elseif first == '+' then
    value = string.sub(value, 2)
  end
  value = string.gsub(value, '^0+', '')
  if value == '' then value = '0' end
  if value == '0' then return '0' end
  return sign .. value
end
local function compare(a, b)
  a = normalize(a)
  b = normalize(b)
  local aNegative = string.sub(a, 1, 1) == '-'
  local bNegative = string.sub(b, 1, 1) == '-'
  if aNegative and not bNegative then return -1 end
  if not aNegative and bNegative then return 1 end
  local aa = aNegative and string.sub(a, 2) or a
  local bb = bNegative and string.sub(b, 2) or b
  if string.len(aa) < string.len(bb) then return aNegative and 1 or -1 end
  if string.len(aa) > string.len(bb) then return aNegative and -1 or 1 end
  if aa == bb then return 0 end
  if aa < bb then return aNegative and 1 or -1 end
  return aNegative and -1 or 1
end
local remain = redis.call('HGET', KEYS[1], 'RemainQuota')
if remain == false or compare(remain, ARGV[1]) < 0 then
  return 0
end
local negativeAmount = '-' .. ARGV[1]
local ok = redis.pcall('HINCRBY', KEYS[1], 'RemainQuota', negativeAmount)
if type(ok) == 'table' and ok.err then return -2 end
local used = redis.pcall('HINCRBY', KEYS[1], 'UsedQuota', ARGV[1])
if type(used) == 'table' and used.err then
  redis.call('HINCRBY', KEYS[1], 'RemainQuota', ARGV[1])
  return -2
end
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

const tokenQuotaDeltaScript = `
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
local delta = ARGV[1]
local opposite = delta
if string.sub(delta, 1, 1) == '-' then
  opposite = string.sub(delta, 2)
else
  opposite = '-' .. delta
end
local remain = redis.pcall('HINCRBY', KEYS[1], 'RemainQuota', delta)
if type(remain) == 'table' and remain.err then return -2 end
local used = redis.pcall('HINCRBY', KEYS[1], 'UsedQuota', opposite)
if type(used) == 'table' and used.err then
  redis.call('HINCRBY', KEYS[1], 'RemainQuota', opposite)
  return -2
end
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[3])
return 1`

func quotaResultFromLua(result int, err error) (cacheQuotaResult, error) {
	if err != nil {
		return cacheQuotaMiss, err
	}
	switch result {
	case 1:
		return cacheQuotaOK, nil
	case 0:
		return cacheQuotaInsufficient, nil
	default:
		return cacheQuotaMiss, nil
	}
}

func cacheTryReserveUserQuota(userID int, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaReserveScript,
		[]string{getUserCacheKey(userID)}, amount, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyUserQuotaDelta(userID int, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), userQuotaDeltaScript,
		[]string{getUserCacheKey(userID)}, delta, userID, userCacheSchemaVersion).Int()
	return quotaResultFromLua(result, err)
}

func cacheTryReserveTokenQuota(id int, key string, amount int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaReserveScript,
		[]string{getTokenCacheKey(key)}, amount, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

func cacheApplyTokenQuotaDelta(id int, key string, delta int64) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), tokenQuotaDeltaScript,
		[]string{getTokenCacheKey(key)}, delta, id, common.GetTimestamp()).Int()
	return quotaResultFromLua(result, err)
}

// persistUserQuotaDelta 把已在缓存侧预扣成功的增量落库；批量模式下入队，
// 直写模式下要求行存在（用户已删除时报错，交由调用方补偿缓存）。
func persistUserQuotaDelta(id int, delta int64) error {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, delta)
		return nil
	}
	return updateUserQuotaWithDeltaTx(DB, id, delta, nil)
}

func persistTokenQuotaDelta(id int, delta int64) error {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, delta)
		return nil
	}
	return updateTokenQuotaDeltaTx(DB, id, delta)
}

func reserveUserQuotaDB(id int, quota int64) (bool, error) {
	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", id, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	return result.RowsAffected == 1, result.Error
}

func reserveTokenQuotaDB(id int, quota int64) (bool, error) {
	result := DB.Model(&Token{}).
		Where("id = ? AND remain_quota >= ? AND remain_quota >= ? AND used_quota <= ?", id, quota, common.MinWalletQuota+quota, common.MaxWalletQuota-quota).
		Updates(map[string]any{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	var token Token
	if err := DB.Select("id", "remain_quota").Where("id = ?", id).First(&token).Error; err != nil {
		return false, err
	}
	if token.RemainQuota < quota {
		return false, nil
	}
	return false, common.ErrWalletQuotaOverflow
}

// TryReserveUserQuota atomically checks and deducts a user's wallet quota.
// 缓存命中时以缓存余额为准（避免批量模式下过期的数据库余额放大并发超扣）；
// Redis 异常或水合失败时降级为数据库条件更新，保证服务可用。
func TryReserveUserQuota(id int, quota int) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	if !common.RedisEnabled {
		return reserveUserQuotaDB(id, int64(quota))
	}

	result, err := cacheTryReserveUserQuota(id, int64(quota))
	if err == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetUserCache(id); hydrateErr == nil {
			result, err = cacheTryReserveUserQuota(id, int64(quota))
		}
	}
	if err != nil || result == cacheQuotaMiss {
		if err != nil {
			common.SysLog("user quota cache reserve unavailable, falling back to database: " + err.Error())
		}
		return reserveUserQuotaDB(id, int64(quota))
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistUserQuotaDelta(id, -int64(quota)); err != nil {
		compensated, compensateErr := cacheApplyUserQuotaDelta(id, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(fmt.Sprintf("failed to compensate reserved user quota: result=%d error=%v", compensated, compensateErr))
		}
		return false, err
	}
	return true, nil
}

// TryReserveTokenQuota atomically checks and deducts a token quota. Unlimited
// tokens skip the balance check but still update remain/used accounting.
func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	if unlimited {
		return true, DecreaseTokenQuota(id, key, quota)
	}
	if !common.RedisEnabled {
		return reserveTokenQuotaDB(id, int64(quota))
	}

	result, err := cacheTryReserveTokenQuota(id, key, int64(quota))
	if err == nil && result == cacheQuotaMiss {
		if _, hydrateErr := GetTokenByKey(key, true); hydrateErr == nil {
			result, err = cacheTryReserveTokenQuota(id, key, int64(quota))
		}
	}
	if err != nil || result == cacheQuotaMiss {
		if err != nil {
			common.SysLog("token quota cache reserve unavailable, falling back to database: " + err.Error())
		}
		return reserveTokenQuotaDB(id, int64(quota))
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err = persistTokenQuotaDelta(id, -int64(quota)); err != nil {
		compensated, compensateErr := cacheApplyTokenQuotaDelta(id, key, int64(quota))
		if compensateErr != nil || compensated != cacheQuotaOK {
			common.SysError(fmt.Sprintf("failed to compensate reserved token quota: result=%d error=%v", compensated, compensateErr))
		}
		return false, err
	}
	return true, nil
}
