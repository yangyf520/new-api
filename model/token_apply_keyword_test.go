package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenApplyKeywordTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB := DB
	DB = db
	oldSQLite := common.UsingSQLite
	common.UsingSQLite = true
	require.NoError(t, db.AutoMigrate(&TokenApplyRecord{}))
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldSQLite
	})
}

func TestListTokenApplicationsAdminKeywordWorkNo(t *testing.T) {
	setupTokenApplyKeywordTestDB(t)

	records := []TokenApplyRecord{
		{
			Id:       501,
			TicketNo: "WO-501",
			WorkNo:   "10086",
			UserName: "张三",
			OrgCode:  "D001",
			Status:   TokenApplyStatusIssued,
		},
		{
			Id:       502,
			TicketNo: "WO-502",
			WorkNo:   "E20001",
			UserName: "李四",
			OrgCode:  "D001",
			Status:   TokenApplyStatusIssued,
		},
	}
	require.NoError(t, DB.Create(&records).Error)

	t.Run("numeric work_no partial", func(t *testing.T) {
		items, total, err := ListTokenApplicationsAdmin("10086", 0, 20)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		require.Equal(t, 501, items[0].Id)
		require.Equal(t, "10086", items[0].WorkNo)
	})

	t.Run("alphanumeric work_no partial", func(t *testing.T) {
		items, total, err := ListTokenApplicationsAdmin("20001", 0, 20)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		require.Equal(t, 502, items[0].Id)
	})

	t.Run("work_no prefix", func(t *testing.T) {
		items, total, err := ListTokenApplicationsAdmin("E200", 0, 20)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		require.Equal(t, "E20001", items[0].WorkNo)
	})
}
