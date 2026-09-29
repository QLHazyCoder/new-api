package model

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createReserveTestUser(t *testing.T, quota int64) User {
	t.Helper()
	user := User{
		Username:    "reserve-user-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       quota,
		AffCode:     "reserve-aff-" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func createReserveTestToken(t *testing.T, remainQuota int64) Token {
	t.Helper()
	token := Token{
		UserId:      1,
		Key:         "reserve-token-" + common.GetRandomString(8),
		Name:        "reserve-test",
		Status:      common.TokenStatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	require.NoError(t, token.Insert())
	return token
}

func getUserQuotaFromDB(t *testing.T, id int) int64 {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, id).Error)
	return user.Quota
}

func getTokenFromDB(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func resetBatchUpdateTestState(t *testing.T) {
	t.Helper()
	oldBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchEnabled
	})
}

func TestTryReserveQuotaWithoutRedis(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 100)
	reserved, err := TryReserveUserQuota(user.Id, 60)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.EqualValues(t, 40, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 41)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.EqualValues(t, 40, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 80)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 25, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	assert.EqualValues(t, 55, reloaded.RemainQuota)
	assert.EqualValues(t, 25, reloaded.UsedQuota)

	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 56, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.EqualValues(t, 55, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestUnlimitedTokenStillChecksSignedInt64Bounds(t *testing.T) {
	truncateTables(t)
	token := createReserveTestToken(t, math.MinInt64)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 1, true)
	require.False(t, reserved)
	require.ErrorIs(t, err, common.ErrWalletQuotaOverflow)
	require.Equal(t, int64(math.MinInt64), getTokenFromDB(t, token.Id).RemainQuota)

	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": 0, "used_quota": int64(math.MaxInt64),
	}).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 1, true)
	require.False(t, reserved)
	require.ErrorIs(t, err, common.ErrWalletQuotaOverflow)
}

func TestDatabaseReserveSupportsInt64WalletBalancesWithRedisEnabled(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 50_000_000_000)
	require.NoError(t, populateUserCache(user))
	reserved, err := TryReserveUserQuota(user.Id, 1_000)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.EqualValues(t, 49_999_999_000, getUserQuotaFromDB(t, user.Id))
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 49_999_999_000, cachedUser.Quota)

	token := createReserveTestToken(t, 50_000_000_000)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 1_000, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.EqualValues(t, 49_999_999_000, reloadedToken.RemainQuota)
	assert.EqualValues(t, 1_000, reloadedToken.UsedQuota)
}

func TestBatchFlagCannotSpendStaleDatabaseOrCacheBalance(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, 10)
	reserved, err := TryReserveUserQuota(user.Id, 8)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.EqualValues(t, 2, getUserQuotaFromDB(t, user.Id), "reservations commit synchronously")

	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved, "stale DB balance must not authorize a second spend")
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cachedUser.Quota)

	token := createReserveTestToken(t, 9)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	require.NoError(t, err)
	assert.True(t, reserved)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 3, false)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.EqualValues(t, 2, getTokenFromDB(t, token.Id).RemainQuota)

	assert.EqualValues(t, 2, getUserQuotaFromDB(t, user.Id))
	reloadedToken := getTokenFromDB(t, token.Id)
	assert.EqualValues(t, 2, reloadedToken.RemainQuota)
	assert.EqualValues(t, 7, reloadedToken.UsedQuota)
}

func TestBatchUpdateAccumulatesTwoMaximumRequestCharges(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	common.BatchUpdateEnabled = true

	user := createReserveTestUser(t, common.MaxQuota*2+100)
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))
	require.NoError(t, DecreaseUserQuota(user.Id, common.MaxQuota, false))

	assert.EqualValues(t, 100, getUserQuotaFromDB(t, user.Id))
}

func TestStaleCacheCannotAuthorizeOrDenyWalletSpend(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 2)
	require.NoError(t, populateUserCache(user))
	server.HSet(getUserCacheKey(user.Id), "Quota", "10")

	reserved, err := TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.EqualValues(t, 2, getUserQuotaFromDB(t, user.Id))

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 10).Error)
	server.HSet(getUserCacheKey(user.Id), "Quota", "1")
	reserved, err = TryReserveUserQuota(user.Id, 3)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.EqualValues(t, 7, getUserQuotaFromDB(t, user.Id))
}

func TestReserveFallsBackToDatabaseWhenRedisIsUnavailable(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 20)
	require.NoError(t, populateUserCache(user))
	server.Close()

	// Redis 故障时降级为数据库条件更新：服务保持可用且不会超扣。
	reserved, err := TryReserveUserQuota(user.Id, 5)
	require.NoError(t, err)
	assert.True(t, reserved)
	assert.EqualValues(t, 15, getUserQuotaFromDB(t, user.Id))

	reserved, err = TryReserveUserQuota(user.Id, 16)
	require.NoError(t, err)
	assert.False(t, reserved)
	assert.EqualValues(t, 15, getUserQuotaFromDB(t, user.Id))
}

func TestReserveDoesNotTouchCacheWhenDatabaseRowIsMissing(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 10)
	require.NoError(t, populateUserCache(user))
	require.NoError(t, DB.Delete(&user).Error)

	reserved, err := TryReserveUserQuota(user.Id, 6)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, cacheErr := cacheGetUserBase(user.Id)
	require.NoError(t, cacheErr)
	assert.False(t, common.RDB.HExists(t.Context(), getUserCacheKey(user.Id), "Quota").Val())

	token := createReserveTestToken(t, 12)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&token).Error)
	reserved, err = TryReserveTokenQuota(token.Id, token.Key, 7, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	cachedToken, cacheErr := cacheGetTokenByKey(token.Key)
	require.NoError(t, cacheErr)
	assert.Equal(t, token.Id, cachedToken.Id)
	assert.False(t, common.RDB.HExists(t.Context(), getTokenCacheKey(token.Key), "RemainQuota").Val())
}

func TestTokenMetadataCacheDropsQuotaAndFenceBlocksStaleSnapshot(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)

	token := createReserveTestToken(t, 100)
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	stale := *loaded

	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 70, false)
	require.NoError(t, err)
	require.True(t, reserved)

	// Old snapshots cannot restore financial fields into a metadata-only hash.
	code, err := cacheInitToken(stale)
	require.NoError(t, err)
	assert.Equal(t, 2, code)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, token.Id, cached.Id)
	assert.False(t, common.RDB.HExists(t.Context(), getTokenCacheKey(token.Key), "RemainQuota").Val())

	// 变更期间：fence 删除缓存并拦截并发读者手中的过期快照。
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	code, err = cacheInitToken(stale)
	require.NoError(t, err)
	assert.Zero(t, code, "the pre-mutation snapshot must not be published while fenced")
	_, err = cacheGetTokenByKey(token.Key)
	assert.Error(t, err)

	// fence 过期后可重新从数据库水合。
	server.FastForward(time.Duration(tokenCacheFenceSeconds+1) * time.Second)
	fresh, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.EqualValues(t, 30, fresh.RemainQuota)
	cached, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, token.Id, cached.Id)
	assert.False(t, common.RDB.HExists(t.Context(), getTokenCacheKey(token.Key), "UsedQuota").Val())
}
