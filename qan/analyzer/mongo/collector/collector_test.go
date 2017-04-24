package collector

import (
	"reflect"
	"testing"

	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	"github.com/percona/pmgo"
	"github.com/stretchr/testify/assert"
	"fmt"
	"gopkg.in/mgo.v2/bson"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := pmgo.ParseURL("127.0.0.1:27017")

	type args struct {
		dialInfo *pmgo.DialInfo
		dialer   pmgo.Dialer
	}
	tests := []struct {
		name string
		args args
		want *Collector
	}{
		{
			name: "127.0.0.1:27017",
			args: args{
				dialInfo: dialInfo,
				dialer:   dialer,
			},
			want: &Collector{
				dialInfo: dialInfo,
				dialer:   dialer,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.args.dialInfo, tt.args.dialer); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New(%v, %v) = %v, want %v", tt.args.dialInfo, tt.args.dialer, got, tt.want)
			}
		})
	}
}

func TestCollector_StartStop(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := pmgo.ParseURL("127.0.0.1:27017")

	collector1 := New(dialInfo, dialer)
	docsChan, err := collector1.Start()
	assert.Nil(t, err)
	assert.NotNil(t, docsChan)

	defer collector1.Stop()
}

func TestCollector_Stop(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := pmgo.ParseURL("127.0.0.1:27017")

	// #1
	notStarted := New(dialInfo, dialer)

	// #2
	started := New(dialInfo, dialer)
	_, err := started.Start()
	assert.Nil(t, err)

	tests := []struct {
		name string
		self *Collector
	}{
		{
			name: "not started",
			self: notStarted,
		},
		{
			name: "started",
			self: started,
		},
		// repeat to be sure Stop() is idempotent
		{
			name: "not started",
			self: notStarted,
		},
		{
			name: "started",
			self: started,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.self.Stop()
		})
	}
}
type Person struct {
	ID        bson.ObjectId `bson:"_id,omitempty"`
	Name      string
	Phone     string
	Timestamp time.Time
}
func TestCollector(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := pmgo.ParseURL("127.0.0.1:27017")

	collector := New(dialInfo, dialer)
	docsChan, err := collector.Start()
	assert.Nil(t, err)
	defer collector.Stop()

	people := []map[string]string{
		{"name": "Kamil"},
		{"name": "Carlos"},
	}
	go func() {
		session, err := dialer.DialWithInfo(dialInfo)
		assert.Nil(t, err)
		for range people {
			result := Person{}
			err = session.DB("test").C("people").Find(bson.M{"name": "Ale"}).Select(bson.M{"phone": 0}).One(&result)
			assert.Nil(t, err)
		}
	}()

	actual := []proto.SystemProfile{}
	for doc := range docsChan {
		if doc.Ns == "test.people" {
			actual = append(actual, doc)
			j, _ := bson.Marshal(doc.Query)
			fmt.Println(string(j))
		}
		if len(actual) == len(people) {
			// stopping collector should also close docsChan
			collector.Stop()
		}
	}
	assert.Len(t, actual, len(people))
	assert.Equal(t, "", actual[0].Query)
}