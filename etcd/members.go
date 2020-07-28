package etcd

import (
	"os"
	"strings"
	"context"
	"github.com/Masterminds/cookoo"
	"github.com/Masterminds/cookoo/log"
	//"github.com/coreos/etcd/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"github.com/coreos/etcd/clientv3"
)

// AddMember Add a new member to the cluster.
//
// Conceptually, this is equivalent to `etcdctl member add NAME IP`.
//
// Params:
// 	- client(client.Client): An etcd client
// 	- name (string): The name of the member to add.
// 	- url (string): The peer ip:port or domain: port to use.
//
// Returns:
//	An etcd *client.Member.
func AddMember(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli := p.Get("client", nil).(clientv3.Client)
	name := p.Get("name", "default").(string)
	addr := p.Get("url", "127.0.0.1:2380").(string)
	cluster := clientv3.NewCluster(&cli)
	var addrs []string
	addrs = append(addrs, addr)
	resp, err := cluster.MemberAdd(dctx(), addrs)
	if err != nil {
		log.Errf(c, "Failed to add %s to cluster: %s", addr, err)
		return nil, err
	}

	log.Infof(c, "Added %s (%s) to cluster", addr, resp.Member.ID)

	resp.Member.Name = name

	return resp.Member, nil
}

// RemoveMemberByName removes a member whose name matches the given.
//
// Params:
// 	- client(client.Client): An etcd client
//	- name (string): The name to remove
// Returns:
//	true if the member was found, false otherwise.
func RemoveMemberByName(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli := p.Get("client", nil).(clientv3.Client)
	name := p.Get("name", "____").(string)
	cluster := clientv3.NewCluster(&cli)

	members, err := cluster.MemberList(dctx())
	if err != nil {
		log.Errf(c, "Could not get a list of members: %s", err)
		return false, err
	}

	remIDs := []uint64{}
	for _, member := range members.Members {
		if member.Name == name {
			log.Infof(c, "Removing member %s (ID: %s)", name, member.ID)
			// If this is synchronizable, we should do it in parallel.
			if _, err  := cluster.MemberRemove(dctx(), member.ID); err != nil {
				log.Errf(c, "Failed to remove member: %s", err)
				return len(remIDs) > 0, err
			}
			remIDs = append(remIDs, member.ID)
		}
	}

	return len(remIDs) > 0, nil
}

// RemoveStaleMembers deletes cluster members whose pods are no longer running.
//
// This queries Kubernetes to determine what etcd pods are running, and then
// compares that to the member list in the etcd cluster. It removes any
// cluster members who are no longer in the pod list.
//
// The purpose of this is to keep the cluster membership from deadlocking
// when inactive members prevent consensus building.
//
// Params:
//	- client (etcd/client.Client): The etcd client
// 	- label (string): The pod label indicating an etcd node
// 	- namespace (string): The namespace we're operating in
func RemoveStaleMembers(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	eclient := p.Get("client", nil).(clientv3.Client)
	label := p.Get("label", "name=drycc-etcd-1").(string)
	ns := p.Get("namespace", "default").(string)

	// Should probably pass in the client from the context.
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Errf(c, "Could not get Kubernetes in-cluster config: %s", err)
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Errf(c, "Could not create a Kubernetes client: %s", err)
		return nil, err
	}

	mapi := clientv3.Cluster(eclient)

	members := map[string]bool{}
	idmap := map[string]uint64{}

	// Get members from etcd
	mm, err := mapi.MemberList(dctx())
	if err != nil {
		log.Warnf(c, "Could not get a list of etcd members: %s", err)
		return nil, err
	}
	for _, member := range mm.Members {
		members[member.Name] = false
		idmap[member.Name] = member.ID
	}

	// Get the pods running with the given label
	labelSelector, err := labels.Parse(label)
	if err != nil {
		log.Errf(c, "Selector failed to parse: %s", err)
		return nil, err
	}
	pods, err := clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}

	for _, item := range pods.Items {
		if _, ok := members[item.Name]; !ok {
			log.Infof(c, "Etcd pod %s is not in cluster yet.", item.Name)
		} else {
			members[item.Name] = true
		}
	}

	// Anything marked false in members should be removed from etcd.
	deleted := 0
	for k, v := range members {
		if !v {
			log.Infof(c, "Deleting %s (%s) from etcd cluster members", k, idmap[k])
			if _, err := mapi.MemberRemove(dctx(), idmap[k]); err != nil {
				log.Errf(c, "Failed to remove %s from cluster. Skipping. %s", k, err)
			} else {
				deleted++
			}
		}
	}

	return deleted, nil
}

// GetInitialCluster gets the initial cluster members.
//
// When adding a new node to a cluster, Etcd requires that you pass it
// a list of initial members, in the form "MEMBERNAME=URL". This command
// generates that list and puts it into the environment variable
// ETCD_INITIAL_CLUSTER
//
// Params:
// 	client (client.Client): An etcd client.
// Returns:
//  string representation of the list, also put into the environment.
func GetInitialCluster(c cookoo.Context, p *cookoo.Params) (interface{}, cookoo.Interrupt) {
	cli := p.Get("client", nil).(clientv3.Client)
	mem := clientv3.Cluster(&cli)

	members, err := mem.MemberList(dctx())
	if err != nil {
		return "", err
	}

	b := []string{}
	for _, member := range members.Members {
		for _, purl := range member.PeerURLs {
			if member.Name == "" {
				member.Name = os.Getenv("HOSTNAME")
			}
			b = append(b, member.Name+"="+purl)
		}
	}

	ic := strings.Join(b, ",")
	log.Infof(c, "ETCD_INITIAL_CLUSTER=%s", ic)
	os.Setenv("ETCD_INITIAL_CLUSTER", ic)
	return ic, nil
}
