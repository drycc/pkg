package etcd

// EVERYTHING IN THIS FILE IS DEPRECATED.
// It is here only for compatibility with older code, and will be removed
// as soon as we can purge builder.

import (
	"fmt"
	//"time"

	"github.com/Masterminds/cookoo"
	"github.com/coreos/etcd/clientv3"
)

// Getter describes the Get behavior of an Etcd client.
//
// Usually you will want to use go-etcd/etcd.Client to satisfy this.
//
// We use an interface because it is more testable.
type Getter interface {
	Get(string, bool, bool) (*clientv3.GetResponse, error)
}

// DirCreator describes etcd's CreateDir behavior.
//
// Usually you will want to use go-etcd/etcd.Client to satisfy this.
//type DirCreator interface {
//	CreateDir(string, uint64) (*client.Response, error)
//}

// Setter sets a value in Etcd.
type Setter interface {
	Set(string, string, uint64) (*clientv3.PutResponse, error)
}

// GetterSetter performs get and set operations.
type GetterSetter interface {
	Getter
	Setter
}

// CreateSimpleClient creates a legacy simple client.
//
// DO NOT USE unless you must for backward compatibility.
//
// Params:
// 	- url (string): A server to connect to. This runs through os.ExpandEnv().
// 	- retries (int): Number of times to retry a connection to the server
// 	- retrySleep (time.Duration): How long to sleep between retries
//
// Returns:
// 	This puts a SimpleEtcdClient into context (implements Getter, Setter, etc.)
func CreateSimpleClient(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	r, e := CreateClient(c, p)
	if e != nil {
		return c, e
	}

	return &SimpleEtcdClient{
		realClient: r.(clientv3.Client),
	}, nil
}

// NewSimpleClient Provides a simple wrapper around the old API.
//
// DO NOT USE for new code. Instead, use NewClient().
func NewSimpleClient(hosts []string) (*SimpleEtcdClient, error) {
	cfg := clientv3.Config{
		Endpoints: hosts,
	}

	r, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}

	return &SimpleEtcdClient{
		realClient: *r,
	}, nil
}

// SimpleEtcdClient provides an interface compatible with the old Etcd client.
type SimpleEtcdClient struct {
	realClient clientv3.Client
}

// Get clientv3.GetResponse
func (c *SimpleEtcdClient) Get(key string, sort bool, rec bool) (*clientv3.GetResponse, error) {
	k := clientv3.NewKV(&c.realClient)
	if rec {
		return k.Get(dctx(), key, clientv3.WithPrefix())
	} else {
		return k.Get(dctx(), key)
	}

}

// Set clientv3.GetResponse
func (c *SimpleEtcdClient) Set(key, val string, ttl uint64) (*clientv3.PutResponse, error) {
	l := clientv3.Lease(&c.realClient)
	k := clientv3.NewKV(&c.realClient)
	// We're banking on people not using really uge ttls. In the code base, the
	// highest is only a few hundred.
	if ttl > 0 {
		leaseGrantResp, err := l.Grant(dctx(), int64(ttl))
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		opts := []clientv3.OpOption{clientv3.WithLease(leaseGrantResp.ID)}
		return k.Put(dctx(), key, val, opts...)
	} else {
		return k.Put(dctx(), key, val)
	}

}

// clientv2
// CreateDir by name
//func (c *SimpleEtcdClient) CreateDir(name string, ttl uint64) (*client.Response, error) {
//	k := client.NewKeysAPI(c.realClient)
//	return k.Set(dctx(), name, "", &client.SetOptions{TTL: time.Duration(ttl) * time.Second, Dir: true})
//}
