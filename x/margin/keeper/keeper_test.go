package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"

	clptest "github.com/Sifchain/sifnode/x/clp/test"
	"github.com/Sifchain/sifnode/x/margin/test"
	"github.com/Sifchain/sifnode/x/margin/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestKeeper_Errors(t *testing.T) {
	_, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
}

type MyMockedMTP struct {
	mock.Mock
}

func TestKeeper_SetMTP_NoAsset(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	
	mtp := new(MyMockedMTP)
	
	err := marginKeeper.SetMTP(ctx, mtp)
	assert.EqualError(t, err, "no asset specified: mtp invalid")
}

func TestKeeper_SetMTP_NoAddress(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	
	mtp := types.NewMTP()
	mtp.CollateralAsset = "xxx"
	
	err := marginKeeper.SetMTP(ctx, &mtp)
	assert.EqualError(t, err, "no address specified: mtp invalid")
}

func TestKeeper_SetMTP(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	
	mtp := types.NewMTP()
	mtp.CollateralAsset = "xxx"
	mtp.Address = "xxx"
	
	err := marginKeeper.SetMTP(ctx, &mtp)
	assert.NoError(t, err)
}

func TestKeeper_GetMTP(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.GetMTP(ctx, "xxx", "xxx")
}

func TestKeeper_GetMTPIterator(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.GetMTPIterator(ctx)
}

func TestKeeper_GetMTPs(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.GetMTPs(ctx)
}

func TestKeeper_GetMTPsForAsset(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.GetMTPsForAsset(ctx, "xxx")
}

func TestKeeper_GetAssetsForMTP(t *testing.T) {
	_, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
}

func TestKeeper_DestroyMTP(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.DestroyMTP(ctx, "xxx", "xxx")
}

func TestKeeper_ClpKeeper(t *testing.T) {
	_, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.ClpKeeper()
}

func TestKeeper_BankKeeper(t *testing.T) {
	_, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.BankKeeper()
}

func TestKeeper_GetLeverageParam(t *testing.T) {
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	marginKeeper.GetLeverageParam(ctx)
}

func TestKeeper_CustodySwap(t *testing.T) {
	pool := clptest.GenerateRandomPool(1)[0]
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	swapResult, err := marginKeeper.CustodySwap(ctx, pool, "xxx", sdk.NewUint(10000))
	assert.NotNil(t, swapResult)
	assert.Error(t, err)
}

func TestKeeper_Borrow(t *testing.T) {
	pool := clptest.GenerateRandomPool(1)[0]
	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	mtp := types.NewMTP()

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	mtp.CollateralAmount = sdk.NewUint(0)
	mtp.LiabilitiesP = sdk.NewUint(0)
	mtp.LiabilitiesI = sdk.NewUint(0)
	mtp.CustodyAmount = sdk.NewUint(0)
	// FIX

	err := marginKeeper.Borrow(ctx, "xxx", sdk.NewUint(10000), sdk.NewUint(1000), mtp, pool, sdk.NewUint(1))
	assert.Error(t, err)
}

func TestKeeper_UpdatePoolHealth(t *testing.T) {
	pool := clptest.GenerateRandomPool(1)[0]

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	pool.ExternalLiabilities = sdk.NewUint(0)
	pool.NativeLiabilities = sdk.NewUint(0)
	// FIX

	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)
	err := marginKeeper.UpdatePoolHealth(ctx, pool)
	assert.Nil(t, err)
}

func TestKeeper_UpdateMTPHealth(t *testing.T) {
	pool := clptest.GenerateRandomPool(1)[0]

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	pool.ExternalLiabilities = sdk.NewUint(0)
	pool.NativeLiabilities = sdk.NewUint(0)
	// FIX

	mtp := types.NewMTP()

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	mtp.CollateralAmount = sdk.NewUint(0)
	mtp.LiabilitiesP = sdk.NewUint(0)
	mtp.LiabilitiesI = sdk.NewUint(0)
	mtp.CustodyAmount = sdk.NewUint(0)
	// FIX

	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)

	_, err := marginKeeper.UpdateMTPHealth(ctx, mtp, pool)
	assert.Error(t, err)
}

func TestKeeper_TestInCustody(t *testing.T) {
	pool := clptest.GenerateRandomPool(1)[0]

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	pool.ExternalLiabilities = sdk.NewUint(0)
	pool.NativeLiabilities = sdk.NewUint(0)
	pool.NativeCustody = sdk.NewUint(0)
	// FIX

	mtp := types.NewMTP()

	// FIX: uninitiated fields (throws SIGSEGV error otherwise)
	mtp.CollateralAmount = sdk.NewUint(0)
	mtp.LiabilitiesP = sdk.NewUint(0)
	mtp.LiabilitiesI = sdk.NewUint(0)
	mtp.CustodyAmount = sdk.NewUint(0)
	// FIX

	ctx, app := test.CreateTestAppMargin(false)
	marginKeeper := app.MarginKeeper
	assert.NotNil(t, marginKeeper)

	err := marginKeeper.TakeInCustody(ctx, mtp, pool)
	assert.Nil(t, err)
}