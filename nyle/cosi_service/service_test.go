package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cothority/v3"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
)

var tSuite = cothority.Suite

func init() {
	nyleSID = onet.ServiceFactory.ServiceID(Name)
}

var nyleSID onet.ServiceID

func TestMain(m *testing.M) {
	log.MainTest(m)
}

func TestService_DummyTx(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	_, _, genService := local.MakeSRS(tSuite, 4, nyleSID)
	defer local.CloseAll()

	service := genService.(*Service)
	_, err := service.AddTx(dummyTx())
	require.NoError(t, err)
}

func TestService_BFTCoSiSimul(t *testing.T) {
	local := onet.NewTCPTest(tSuite)
	_, roster, genService := local.MakeSRS(tSuite, 4, nyleSID)
	defer local.CloseAll()

	service := genService.(*Service)
	reply, err := service.BFTCoSiSimul(&BFTCoSiSimul{
		Roster: *roster,
	})
	require.NoError(t, err)
	require.True(t, reply.Duration > 0)
}

func dummyTx() *AddTxReq {
	return &AddTxReq{}
}
