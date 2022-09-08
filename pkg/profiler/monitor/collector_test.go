package monitor

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/redis"
	"cto-github.cisco.com/NFV-BU/go-lanai/test"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/apptest"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/embedded"
	"cto-github.cisco.com/NFV-BU/go-lanai/test/suitetest"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
	"testing"
	"time"
)

/*************************
	Setup
 *************************/

// TestMain is the only place we should kick off embedded redis
func TestMain(m *testing.M) {
	suitetest.RunTests(m,
		embedded.Redis(),
	)
}

/*************************
	Test
 *************************/

type testDI struct {
	fx.In
	DataCollector *dataCollector
}

func TestLogoutMiddleware(t *testing.T) {
	SamplingRate = 100 * time.Microsecond
	di := &testDI{}
	test.RunTest(context.Background(), t,
		apptest.Bootstrap(),
		apptest.WithModules(Module, redis.Module),
		apptest.WithDI(di),
		test.GomegaSubTest(SubTestSubscribe(di), "TestSubscribe"),
	)
}

/*************************
	Sub Tests
 *************************/

func SubTestSubscribe(di *testDI) test.GomegaSubTestFunc {
	return func(ctx context.Context, t *testing.T, g *gomega.WithT) {
		ch, id, e := di.DataCollector.Subscribe()
		g.Expect(e).To(Succeed())
		g.Expect(id).To(Not(BeEmpty()))
		g.Expect(ch).To(Not(BeNil()))

		for i := 0; i < 10; i++ {
			select {
			case f := <-ch:
				g.Expect(f).To(Not(BeZero()))
			case <-ctx.Done():
			}
		}
		di.DataCollector.Unsubscribe(id)
	}
}