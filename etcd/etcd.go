/*Package etcd is a library for performing common Etcd tasks.
 */
package etcd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"
	//"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
	"github.com/Masterminds/cookoo"
	"github.com/Masterminds/cookoo/log"
	"github.com/Masterminds/cookoo/safely"
	"github.com/coreos/etcd/clientv3"
)

// dctx returns a default context for simpleEtcdClient.
func dctx() context.Context {
	// TODO: Add a sensible timeout. 20 seconds? 5 seconds?
	return context.Background()
}

// CreateClient creates a new Etcd client and prepares it for work.
//
// Params:
// 	- url (string): A server to connect to. This runs through os.ExpandEnv().
// 	- retries (int): Number of times to retry a connection to the server
// 	- retrySleep (time.Duration): How long to sleep between retries
//
// Returns:
// 	This puts an *etcd.Client into the context.
func CreateClient(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	url := p.Get("url", "http://localhost:4001").(string)
	url = os.ExpandEnv(url)

	// Backed this out because it's unnecessary so far.
	//hosts := p.Get("urls", []string{"http://localhost:4001"}).([]string)
	hosts := []string{url}

	// Support `host:port` format, too.
	for i, host := range hosts {
		if !strings.Contains(host, "://") {
			hosts[i] = "http://" + host
		}
	}
	cfg := clientv3.Config{
		Endpoints: hosts,
	}

	log.Infof(c, "Client configured for Etcd servers '%s'", strings.Join(hosts, ","))

	return clientv3.New(cfg)
}

// SimpleGet performs the common base-line get, using a default context.
//
// This can be used in cases where no special contextual concerns apply.
func SimpleGet(cli clientv3.Client, key string, recursive bool) (*clientv3.GetResponse, error) {
	k := clientv3.NewKV(&cli)
	if recursive {
		return k.Get(dctx(), key, clientv3.WithPrefix())
	} else {
		return k.Get(dctx(), key)
	}
}

// SimpleSet performs the common base-line set, using a default context.
func SimpleSet(cli clientv3.Client, key, value string, expires time.Duration) (*clientv3.PutResponse, error) {
	opts, err := getTTLOption(cli, int64(expires))
	if err != nil {
		return nil, err
	}
	putResp, err := cli.Put(dctx(), key, value, opts...)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return putResp, nil
}

// Get performs an etcd Get operation.
//
// Params:
// 	- client (EtcdGetter): Etcd client
// 	- path (string): The path/key to fetch
// 	- recursive (bool): Get children, too. Default: false.
// 	- sort (bool): Lexigraphically sort by name. Default: false.
//
// Returns:
// - This puts an `etcd.Response` into the context, and returns an error
//   if the client could not connect.
func Get(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli, ok := p.Has("client")
	if !ok {
		return nil, errors.New("no etcd client found")
	}
	ec := cli.(clientv3.Client)
	path := p.Get("path", "/").(string)
	rec := p.Get("recursive", false).(bool)
	sort := p.Get("sort", false).(bool)
	ops := []clientv3.OpOption{}
	if rec {
		ops = append(ops, clientv3.WithPrefix())
	}
	if sort {
		ops = append(ops, clientv3.WithSort(0,1))
	}
	k := clientv3.NewKV(&ec)

	res, err := k.Get(dctx(), path,  ops...)
	if err != nil {
		return res, err
	}
	if len(res.Kvs) == 0  {
		return res, fmt.Errorf("No results returned from etcdv3 client")
	}
	return res, nil
}

// IsRunning checks to see if etcd is running.
//
// It will test `count` times before giving up.
//
// Params:
// 	- client (EtcdGetter)
// 	- count (int): Number of times to try before giving up.
//
// Returns:
// 	boolean true if etcd is listening.
func IsRunning(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli := p.Get("client", nil).(clientv3.Client)
	count := p.Get("count", 20).(int)
	k := clientv3.NewKV(&cli)
	for i := 0; i < count; i++ {
		_, err := k.Get(dctx(), "/")
		if err == nil {
			return true, nil
		}
		log.Infof(c, "Waiting for etcd to come online.")
		time.Sleep(250 * time.Millisecond)
	}
	log.Errf(c, "Etcd is not answering after %d attempts.", count)
	return false, &cookoo.FatalError{Message: "Could not connect to Etcd."}
}

// Set sets a value in etcd.
//
// Params:
// 	- key (string): The key
// 	- value (string): The value
// 	- ttl (uint64): Time to live
// 	- client (EtcdGetter): Client, usually an *etcd.Client.
//
// Returns:
// 	- *etcd.Result
func Set(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	key := p.Get("key", "").(string)
	value := p.Get("value", "").(string)
	ttl := p.Get("ttl", uint64(20)).(uint64)
	cli := p.Get("client", nil).(clientv3.Client)
	opts, err := getTTLOption(cli, int64(ttl))
	if err != nil {
		return nil, err
	}
	res, err := cli.Put(dctx(), key, value, opts...)
	if err != nil {
		log.Infof(c, "Failed to set %s=%s", key, value)
		return res, err
	}
	return res, nil
}

// FindSSHUser finds an SSH user by public key.
//
// Some parts of the system require that we know not only the SSH key, but also
// the name of the user. That information is stored in etcd.
//
// Params:
// 	- client (EtcdGetter)
// 	- fingerprint (string): The fingerprint of the SSH key.
//
// Returns:
// - username (string)
func FindSSHUser(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli := p.Get("client", nil).(clientv3.Client)
	fingerprint := p.Get("fingerprint", nil).(string)

	k := clientv3.NewKV(&cli)

	res, err := k.Get(dctx(), "/drycc/builder/users")
	if err != nil {
		log.Warnf(c, "Error querying etcd: %s", err)
		return "", err
	} else if len(res.Kvs) == 0 {
		log.Warnf(c, "No users found in etcd.")
		return "", errors.New("Users not found")
	}
	for _, user := range res.Kvs {
		log.Infof(c, "Checking user %s", user.Key)
		if strings.HasSuffix(string(user.Value), fingerprint) {
			parts := strings.Split(string(user.Key), "/")
			username := parts[len(parts)-1]
			log.Infof(c, "Found user %s for fingerprint %s", username, fingerprint)
			return username, nil
		}
	}
	return "", fmt.Errorf("User not found for fingerprint %s", fingerprint)
}

// StoreHostKeys stores SSH hostkeys locally.
//
// First it tries to fetch them from etcd. If the keys are not present there,
// it generates new ones and then puts them into etcd.
//
// Params:
// 	- client(EtcdGetterSetter)
// 	- ciphers([]string): A list of ciphers to generate. Defaults are dsa,
// 		ecdsa, ed25519 and rsa.
// 	- basepath (string): Base path in etcd (ETCD_PATH).
func StoreHostKeys(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	defaultCiphers := []string{"rsa", "dsa", "ecdsa", "ed25519"}
	cli := p.Get("client", nil).(clientv3.Client)
	ciphers := p.Get("ciphers", defaultCiphers).([]string)
	basepath := p.Get("basepath", "/drycc/builder").(string)

	k := clientv3.NewKV(&cli)
	res, err := k.Get(dctx(), "sshHostKey")
	if err != nil || len(res.Kvs) == 0 {
		log.Infof(c, "Could not get SSH host key from etcd. Generating new ones.")
		if err := genSSHKeys(c); err != nil {
			log.Err(c, "Failed to generate SSH keys. Aborting.")
			return nil, err
		}
		if err := keysToEtcd(c, k, ciphers, basepath); err != nil {
			return nil, err
		}
	} else if err := keysToLocal(c, k, ciphers, basepath); err != nil {
		log.Infof(c, "Fetching SSH host keys from etcd.")
		return nil, err
	}

	return nil, nil
}

// keysToLocal copies SSH host keys from etcd to the local file system.
//
// This only fails if the main key, sshHostKey cannot be stored or retrieved.
func keysToLocal(c cookoo.Context, k clientv3.KV, ciphers []string, etcdPath string) error {
	lpath := "/etc/ssh/ssh_host_%s_key"
	privkey := "%s/sshHost%sKey"
	for _, cipher := range ciphers {
		path := fmt.Sprintf(lpath, cipher)
		key := fmt.Sprintf(privkey, etcdPath, cipher)
		res, err := k.Get(dctx(), key)
		if err != nil || len(res.Kvs) == 0 {
			continue
		}

		content := res.Kvs[0].Value
		if err := ioutil.WriteFile(path, []byte(content), 0600); err != nil {
			log.Errf(c, "Error writing ssh host key file: %s", err)
		}
	}

	// Now get generic key.
	res, err := k.Get(dctx(), "sshHostKey")
	if err != nil || len(res.Kvs) == 0 {
		return fmt.Errorf("Failed to get sshHostKey from etcd. %v", err)
	}

	content := res.Kvs[0].Value
	if err := ioutil.WriteFile("/etc/ssh/ssh_host_key", []byte(content), 0600); err != nil {
		log.Errf(c, "Error writing ssh host key file: %s", err)
		return err
	}
	return nil
}

// keysToEtcd copies local keys into etcd.
//
// It only fails if it cannot copy ssh_host_key to sshHostKey. All other
// abnormal conditions are logged, but not considered to be failures.
func keysToEtcd(c cookoo.Context, k clientv3.KV, ciphers []string, etcdPath string) error {
	lpath := "/etc/ssh/ssh_host_%s_key"
	privkey := "%s/sshHost%sKey"
	for _, cipher := range ciphers {
		path := fmt.Sprintf(lpath, cipher)
		key := fmt.Sprintf(privkey, etcdPath, cipher)
		content, err := ioutil.ReadFile(path)
		if err != nil {
			log.Infof(c, "No key named %s", path)
		} else if _, err := k.Put(dctx(), key, string(content)); err != nil {
			log.Errf(c, "Could not store ssh key in etcd: %s", err)
		}
	}
	// Now we set the generic key:
	if content, err := ioutil.ReadFile("/etc/ssh/ssh_host_key"); err != nil {
		log.Errf(c, "Could not read the ssh_host_key file.")
		return err
	} else if _, err := k.Put(dctx(), "sshHostKey", string(content)); err != nil {
		log.Errf(c, "Failed to set sshHostKey in etcd.")
		return err
	}
	return nil
}

// genSshKeys generates the default set of SSH host keys.
func genSSHKeys(c cookoo.Context) error {
	// Generate a new key
	out, err := exec.Command("ssh-keygen", "-A").CombinedOutput()
	if err != nil {
		log.Infof(c, "ssh-keygen: %s", out)
		log.Errf(c, "Failed to generate SSH keys: %s", err)
		return err
	}
	return nil
}

// UpdateHostPort intermittently notifies etcd of the builder's address.
//
// If `port` is specified, this will notify etcd at 10 second intervals that
// the builder is listening at $HOST:$PORT, setting the TTL to 20 seconds.
//
// This will notify etcd as long as the local sshd is running.
//
// Params:
// 	- base (string): The base path to write the data: $base/host and $base/port.
// 	- host (string): The hostname
// 	- port (string): The port
// 	- client (Setter): The client to use to write the data to etcd.
// 	- sshPid (int): The PID for SSHD. If SSHD dies, this stops notifying.
func UpdateHostPort(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	base := p.Get("base", "").(string)
	host := p.Get("host", "").(string)
	port := p.Get("port", "").(string)
	cli := p.Get("client", nil).(clientv3.Client)
	sshd := p.Get("sshdPid", 0).(int)

	// If no port is specified, we don't do anything.
	if len(port) == 0 {
		log.Infof(c, "No external port provided. Not publishing details.")
		return false, nil
	}

	ttl := time.Second * 20
	k := clientv3.NewKV(&cli)

	if err := setHostPort(cli, k, base, host, port, ttl); err != nil {
		log.Errf(c, "Etcd error setting host/port: %s", err)
		return false, err
	}

	// Update etcd every ten seconds with this builder's host/port.
	safely.GoDo(c, func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			//log.Infof(c, "Setting SSHD host/port")
			if _, err := os.FindProcess(sshd); err != nil {
				log.Errf(c, "Lost SSHd process: %s", err)
				break
			} else {
				if err := setHostPort(cli, k, base, host, port, ttl); err != nil {
					log.Errf(c, "Etcd error setting host/port: %s", err)
					break
				}
			}
		}
		ticker.Stop()
	})

	return true, nil
}

func setHostPort(c clientv3.Client,k clientv3.KV, base, host, port string, ttl time.Duration) error {
	//o := client.SetOptions{TTL: ttl}
	opts, err := getTTLOption(c, int64(ttl))
	if err != nil {
		return err
	}
	if _, err := k.Put(dctx(), base+"/host", host, opts...); err != nil {
		return err
	}
	if _, err := k.Put(dctx(), base+"/port", port, opts...); err != nil {
		return err
	}
	return nil
}

// MakeDir makes a directory in Etcd.
//
// Params:
// 	- client (EtcdDirCreator): Etcd client
//  - path (string): The name of the directory to create.
// 	- ttl (uint64): Time to live.
// Returns:
// 	*etcd.Response
//  clientv2
//func MakeDir(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
//	name := p.Get("path", "").(string)
//	t := p.Get("ttl", uint64(0)).(uint64)
//	ttl := time.Duration(t) * time.Second
//
//	cli := p.Get("client", nil).(client.Client)
//	k := client.NewKeysAPI(cli)
//
//	if len(name) == 0 {
//		return false, errors.New("expected directory name to be more than zero characters")
//	}
//
//	res, err := k.Set(dctx(), name, "", &client.SetOptions{TTL: ttl, Dir: true})
//	if err != nil {
//		return res, &cookoo.RecoverableError{Message: err.Error()}
//	}
//
//	return res, nil
//}

// Watch watches a given path, and executes a git check-repos for each event.
//
// It starts the watcher and then returns. The watcher runs on its own
// goroutine. To stop the watching, send the returned channel a bool.
//
// Params:
// - client (Watcher): An Etcd client.
// - path (string): The path to watch
func Watch(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {

	// etcdctl -C $ETCD watch --recursive /drycc/services
	path := p.Get("path", "/drycc/services").(string)
	cli := p.Get("client", nil).(clientv3.Client)
	k := clientv3.NewWatcher(&cli)

	watcher := k.Watch(dctx(), path, clientv3.WithPrefix())

	safely.GoDo(c, func() {
		for {
			for watchResp := range watcher {
				for _, watchEvent := range watchResp.Events{
					fmt.Printf("%s,%q,%q",watchEvent.Type,watchEvent.Kv.Key,watchEvent.Kv.Value)
					git := exec.Command("/home/git/check-repos")
					if out, err := git.CombinedOutput(); err != nil {
						log.Errf(c, "Failed git check-repos: %s", err)
						log.Infof(c, "Output: %s", out)
					}
				}
				//// TODO: We should probably add cancellation support.
				//response, err := watcher.Next(dctx())
				//if err != nil {
				//	log.Errf(c, "Etcd Watch failed: %s", err)
				//}
				//if response.Node == nil {
				//	log.Infof(c, "Unexpected Etcd message: %v", response)
				//}
				//git := exec.Command("/home/git/check-repos")
				//if out, err := git.CombinedOutput(); err != nil {
				//	log.Errf(c, "Failed git check-repos: %s", err)
				//	log.Infof(c, "Output: %s", out)
				//}
			}
		}
	})
	return nil, nil
}

func getTTLOption(c clientv3.Client, TTL int64) ([]clientv3.OpOption, error) {
	putOpts := []clientv3.OpOption{}

	if TTL >= 0 {
		resp, err := c.Lease.Grant(dctx(), int64(TTL))
		if err != nil {
			return nil, errors.New("created lease error")
		}
		putOpts = append(putOpts, clientv3.WithLease(resp.ID))
	}
	return putOpts, nil
}