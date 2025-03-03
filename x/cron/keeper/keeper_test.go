package keeper_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/neutron-org/neutron/v2/testutil"
	testutil_keeper "github.com/neutron-org/neutron/v2/testutil/cron/keeper"
	mock_types "github.com/neutron-org/neutron/v2/testutil/mocks/cron/types"
	"github.com/neutron-org/neutron/v2/x/cron/types"
)

// ExecuteReadySchedules:
// - calls msgServer.execute() on ready schedules
// - updates ready schedules lastExecuteHeight
// - does not update lastExecuteHeight of unready schedules
// - does not go over the limit
func TestKeeperExecuteReadySchedules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	accountKeeper := mock_types.NewMockAccountKeeper(ctrl)
	addr, err := sdk.AccAddressFromBech32(testutil.TestOwnerAddress)
	require.NoError(t, err)

	wasmMsgServer := mock_types.NewMockWasmMsgServer(ctrl)
	k, ctx := testutil_keeper.CronKeeper(t, wasmMsgServer, accountKeeper)
	ctx = ctx.WithBlockHeight(0)

	err = k.SetParams(ctx, types.Params{
		SecurityAddress: testutil.TestOwnerAddress,
		Limit:           2,
	})
	require.NoError(t, err)

	schedules := []types.Schedule{
		{
			Name:   "1_unready1",
			Period: 3,
			Msgs: []types.MsgExecuteContract{
				{
					Contract: "1_neutron",
					Msg:      "1_msg",
				},
			},
			LastExecuteHeight: 4,
		},
		{
			Name:   "2_ready1",
			Period: 3,
			Msgs: []types.MsgExecuteContract{
				{
					Contract: "2_neutron",
					Msg:      "2_msg",
				},
			},
			LastExecuteHeight: 0,
		},
		{
			Name:   "3_ready2",
			Period: 3,
			Msgs: []types.MsgExecuteContract{
				{
					Contract: "3_neutron",
					Msg:      "3_msg",
				},
			},
			LastExecuteHeight: 0,
		},
		{
			Name:              "4_unready2",
			Period:            3,
			Msgs:              []types.MsgExecuteContract{},
			LastExecuteHeight: 4,
		},
		{
			Name:   "5_ready3",
			Period: 3,
			Msgs: []types.MsgExecuteContract{
				{
					Contract: "5_neutron",
					Msg:      "5_msg",
				},
			},
			LastExecuteHeight: 0,
		},
	}

	for _, item := range schedules {
		ctx = ctx.WithBlockHeight(int64(item.LastExecuteHeight))
		err := k.AddSchedule(ctx, item.Name, item.Period, item.Msgs)
		require.NoError(t, err)
	}

	count := k.GetScheduleCount(ctx)
	require.Equal(t, count, int32(5))

	ctx = ctx.WithBlockHeight(5)

	accountKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(addr)
	accountKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(addr)
	wasmMsgServer.EXPECT().ExecuteContract(gomock.Any(), &wasmtypes.MsgExecuteContract{
		Sender:   testutil.TestOwnerAddress,
		Contract: "2_neutron",
		Msg:      []byte("2_msg"),
		Funds:    sdk.NewCoins(),
	}).Return(nil, fmt.Errorf("executeerror"))
	wasmMsgServer.EXPECT().ExecuteContract(gomock.Any(), &wasmtypes.MsgExecuteContract{
		Sender:   testutil.TestOwnerAddress,
		Contract: "3_neutron",
		Msg:      []byte("3_msg"),
		Funds:    sdk.NewCoins(),
	}).Return(&wasmtypes.MsgExecuteContractResponse{}, nil)

	k.ExecuteReadySchedules(ctx)

	unready1, _ := k.GetSchedule(ctx, "1_unready1")
	ready1, _ := k.GetSchedule(ctx, "2_ready1")
	ready2, _ := k.GetSchedule(ctx, "3_ready2")
	unready2, _ := k.GetSchedule(ctx, "4_unready2")
	ready3, _ := k.GetSchedule(ctx, "5_ready3")

	require.Equal(t, uint64(4), unready1.LastExecuteHeight)
	require.Equal(t, uint64(5), ready1.LastExecuteHeight)
	require.Equal(t, uint64(5), ready2.LastExecuteHeight)
	require.Equal(t, uint64(4), unready2.LastExecuteHeight)
	require.Equal(t, uint64(0), ready3.LastExecuteHeight)

	// let's make another call at the next height
	// Notice that now only one ready schedule left because we got limit of 2 at once
	ctx = ctx.WithBlockHeight(6)

	accountKeeper.EXPECT().GetModuleAddress(types.ModuleName).Return(addr)
	wasmMsgServer.EXPECT().ExecuteContract(gomock.Any(), &wasmtypes.MsgExecuteContract{
		Sender:   testutil.TestOwnerAddress,
		Contract: "5_neutron",
		Msg:      []byte("5_msg"),
		Funds:    sdk.NewCoins(),
	}).Return(&wasmtypes.MsgExecuteContractResponse{}, nil)

	k.ExecuteReadySchedules(ctx)

	unready1, _ = k.GetSchedule(ctx, "1_unready1")
	ready1, _ = k.GetSchedule(ctx, "2_ready1")
	ready2, _ = k.GetSchedule(ctx, "3_ready2")
	unready2, _ = k.GetSchedule(ctx, "4_unready2")
	ready3, _ = k.GetSchedule(ctx, "5_ready3")

	require.Equal(t, uint64(4), unready1.LastExecuteHeight)
	require.Equal(t, uint64(5), ready1.LastExecuteHeight)
	require.Equal(t, uint64(5), ready2.LastExecuteHeight)
	require.Equal(t, uint64(4), unready2.LastExecuteHeight)
	require.Equal(t, uint64(6), ready3.LastExecuteHeight)
}

func TestAddSchedule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	accountKeeper := mock_types.NewMockAccountKeeper(ctrl)

	wasmMsgServer := mock_types.NewMockWasmMsgServer(ctrl)
	k, ctx := testutil_keeper.CronKeeper(t, wasmMsgServer, accountKeeper)
	ctx = ctx.WithBlockHeight(0)

	err := k.SetParams(ctx, types.Params{
		SecurityAddress: testutil.TestOwnerAddress,
		Limit:           2,
	})
	require.NoError(t, err)

	// normal add schedule
	err = k.AddSchedule(ctx, "a", 7, []types.MsgExecuteContract{
		{
			Contract: "c",
			Msg:      "m",
		},
	})
	require.NoError(t, err)

	// second time with same name returns error
	err = k.AddSchedule(ctx, "a", 5, []types.MsgExecuteContract{})
	require.Error(t, err)

	scheduleA, found := k.GetSchedule(ctx, "a")
	require.True(t, found)
	require.Equal(t, scheduleA.Name, "a")
	require.Equal(t, scheduleA.Period, uint64(7))
	require.Equal(t, scheduleA.Msgs, []types.MsgExecuteContract{
		{Contract: "c", Msg: "m"},
	})

	// remove schedule works
	k.RemoveSchedule(ctx, "a")
	_, found = k.GetSchedule(ctx, "a")
	assert.False(t, found)

	// does not panic even though we don't have it
	k.RemoveSchedule(ctx, "a")
}

func TestGetAllSchedules(t *testing.T) {
	k, ctx := testutil_keeper.CronKeeper(t, nil, nil)

	err := k.SetParams(ctx, types.Params{
		SecurityAddress: testutil.TestOwnerAddress,
		Limit:           2,
	})
	require.NoError(t, err)

	expectedSchedules := make([]types.Schedule, 0, 3)
	for i := range []int{1, 2, 3} {
		s := types.Schedule{
			Name:              strconv.Itoa(i),
			Period:            5,
			Msgs:              nil,
			LastExecuteHeight: uint64(ctx.BlockHeight()),
		}
		expectedSchedules = append(expectedSchedules, s)
		err := k.AddSchedule(ctx, s.Name, s.Period, s.Msgs)
		require.NoError(t, err)
	}

	schedules := k.GetAllSchedules(ctx)
	assert.Equal(t, 3, len(schedules))
	assert.ElementsMatch(t, schedules, expectedSchedules)
	assert.Equal(t, int32(3), k.GetScheduleCount(ctx))
}
