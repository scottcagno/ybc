package memcache

import (
	"bytes"
	"fmt"
	"github.com/valyala/ybc/bindings/go/ybc"
	"testing"
	"time"
)

const (
	testAddr = "localhost:12345"
)

func newCache(t *testing.T) *ybc.Cache {
	config := ybc.Config{
		MaxItemsCount: 1000 * 1000,
		DataFileSize:  10 * 1000 * 1000,
	}

	cache, err := config.OpenCache(true)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func newServerCacheWithAddr(listenAddr string, t *testing.T) (s *Server, cache *ybc.Cache) {
	cache = newCache(t)
	s = &Server{
		Cache:      cache,
		ListenAddr: listenAddr,
	}
	return
}

func newServerCache(t *testing.T) (s *Server, cache *ybc.Cache) {
	return newServerCacheWithAddr(testAddr, t)
}

func TestServer_StartStop(t *testing.T) {
	s, cache := newServerCache(t)
	defer cache.Close()
	s.Start()
	s.Stop()
}

func TestServer_StartStop_Multi(t *testing.T) {
	s, cache := newServerCache(t)
	defer cache.Close()
	for i := 0; i < 3; i++ {
		s.Start()
		s.Stop()
	}
}

func TestServer_Serve(t *testing.T) {
	s, cache := newServerCache(t)
	defer cache.Close()
	go func() {
		time.Sleep(time.Millisecond * time.Duration(100))
		s.Stop()
	}()
	s.Serve()
}

func TestServer_Wait(t *testing.T) {
	s, cache := newServerCache(t)
	defer cache.Close()
	go func() {
		time.Sleep(time.Millisecond * time.Duration(100))
		s.Stop()
	}()
	s.Start()
	s.Wait()
}

func newClientServerCache(t *testing.T) (c *Client, s *Server, cache *ybc.Cache) {
	c = &Client{
		ConnectAddr:      testAddr,
		ConnectionsCount: 1, // tests require single connection!
	}
	s, cache = newServerCache(t)
	s.Start()
	return
}

func newDistributedClientServersCaches(t *testing.T) (c *DistributedClient, ss []*Server, caches []*ybc.Cache) {
	c = &DistributedClient{
		ConnectionsCount: 1, // tests require single connection!
	}
	for i := 0; i < 4; i++ {
		serverAddr := fmt.Sprintf("localhost:%d", 12345+i)
		s, cache := newServerCacheWithAddr(serverAddr, t)
		s.Start()
		ss = append(ss, s)
		caches = append(caches, cache)
	}
	return
}

func cacher_StartStop(c Cacher) {
	c.Start()
	c.Stop()
}

func TestClient_StartStop(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()

	cacher_StartStop(c)
}

func cacher_StartStop_Multi(c Cacher) {
	for i := 0; i < 3; i++ {
		c.Start()
		c.Stop()
	}
}

func TestClient_StartStop_Multi(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()

	cacher_StartStop_Multi(c)
}

func cacher_GetSet(c Cacher, t *testing.T) {
	key := []byte("key")
	value := []byte("value")
	flags := uint32(12345)

	item := Item{
		Key: key,
	}
	if err := c.Get(&item); err != ErrCacheMiss {
		t.Fatalf("Unexpected err=[%s] for client.Get(%s)", err, key)
	}

	item.Value = value
	item.Flags = flags
	if err := c.Set(&item); err != nil {
		t.Fatalf("error in client.Set(): [%s]", err)
	}
	item.Value = nil
	item.Flags = 0
	if err := c.Get(&item); err != nil {
		t.Fatalf("cannot obtain value for key=[%s] from memcache: [%s]", key, err)
	}
	if !bytes.Equal(item.Value, value) {
		t.Fatalf("invalid value=[%s] returned. Expected [%s]", item.Value, value)
	}
	if item.Flags != flags {
		t.Fatalf("invalid flags=[%d] returned. Expected [%d]", item.Flags, flags)
	}
}

func TestClient_GetSet(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_GetSet(c, t)
}

func cacher_GetDe(c Cacher, t *testing.T) {
	item := Item{
		Key: []byte("key"),
	}
	grace := 100 * time.Millisecond
	for i := 0; i < 3; i++ {
		if err := c.GetDe(&item, grace); err != ErrCacheMiss {
			t.Fatalf("Unexpected err=[%s] for client.GetDe(%s, %d)", err, item.Key, grace)
		}
	}

	item.Value = []byte("value")
	if err := c.Set(&item); err != nil {
		t.Fatalf("Cannot set value=[%s] for key=[%s]: [%s]", item.Value, item.Key, err)
	}
	oldValue := item.Value
	item.Value = nil
	if err := c.GetDe(&item, grace); err != nil {
		t.Fatalf("Cannot obtain value fro key=[%s]: [%s]", item.Key, err)
	}
	if !bytes.Equal(oldValue, item.Value) {
		t.Fatalf("Unexpected value obtained: [%s]. Expected [%s]", item.Value, oldValue)
	}
}

func TestClient_GetDe(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_GetDe(c, t)
}

func cacher_CgetCset(c Cacher, t *testing.T) {
	key := []byte("key")
	value := []byte("value")
	expiration := time.Hour * 123343

	etag := uint64(1234567890)
	validateTtl := time.Millisecond * 98765432
	item := Citem{
		Key:         key,
		Value:       value,
		Etag:        etag,
		Expiration:  expiration,
		ValidateTtl: validateTtl,
	}

	if err := c.Cget(&item); err != ErrCacheMiss {
		t.Fatalf("Unexpected error returned from Client.Cget(): [%s]. Expected ErrCacheMiss", err)
	}

	if err := c.Cset(&item); err != nil {
		t.Fatalf("Error in Client.Cset(): [%s]", err)
	}

	if err := c.Cget(&item); err != ErrNotModified {
		t.Fatalf("Unexpected error returned from Client.Cget(): [%s]. Expected ErrNotModified", err)
	}

	item.Value = nil
	item.Etag = 3234898
	item.Expiration = expiration + 10000*time.Second
	if err := c.Cget(&item); err != nil {
		t.Fatalf("Unexpected error returned from Client.Cget(): [%s]", err)
	}
	if item.Etag != etag {
		t.Fatalf("Unexpected etag=[%d] returned from Client.Cget(). Expected [%d]", item.Etag, etag)
	}
	if item.ValidateTtl != validateTtl {
		t.Fatalf("Unexpected validateTtl=[%d] returned from Client.Cget(). Expected [%d]", item.ValidateTtl, validateTtl)
	}
	if !bytes.Equal(item.Value, value) {
		t.Fatalf("Unexpected value=[%s] returned from Client.Cget(). Expected [%d]", item.Value, value)
	}
	if item.Expiration > expiration {
		t.Fatalf("Unexpected expiration=[%d] returned from Client.Cget(). Expected not more than [%d]", item.Expiration, expiration)
	}
}

func TestClient_CgetCset(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_CgetCset(c, t)
}

func lookupItem(items []Item, key []byte) *Item {
	for i := 0; i < len(items); i++ {
		if bytes.Equal(items[i].Key, key) {
			return &items[i]
		}
	}
	return nil
}

func checkItems(c Cacher, orig_items []Item, t *testing.T) {
	keys := make([][]byte, 0, len(orig_items))
	for _, item := range orig_items {
		keys = append(keys, item.Key)
	}

	items, err := c.GetMulti(keys)
	if err != nil {
		t.Fatalf("Error in client.GetMulti(): [%s]", err)
	}
	for _, item := range items {
		orig_item := lookupItem(orig_items, item.Key)
		if orig_item == nil {
			t.Fatalf("Cannot find original item with key=[%s]", item.Key)
		}
		if !bytes.Equal(item.Value, orig_item.Value) {
			t.Fatalf("Values mismatch for key=[%s]. Returned=[%s], expected=[%s]", item.Key, item.Value, orig_item.Value)
		}
	}
}

func checkCItems(c Cacher, items []Citem, t *testing.T) {
	for i := 0; i < len(items); i++ {
		item := items[i]
		err := c.Cget(&item)
		if err == ErrCacheMiss {
			continue
		}
		if err != ErrNotModified {
			t.Fatalf("Unexpected error returned from Client.Cget(): [%s]. Expected ErrNotModified", err)
		}

		item.Etag++
		if err := c.Cget(&item); err != nil {
			t.Fatalf("Error when calling Client.Cget(): [%s]", err)
		}
		if item.Etag != items[i].Etag {
			t.Fatalf("Unexpected etag=%d returned. Expected %d", item.Etag, items[i].Etag)
		}
		if item.ValidateTtl != items[i].ValidateTtl {
			t.Fatalf("Unexpected validateTtl=%d returned. Expected %d", item.ValidateTtl, items[i].ValidateTtl)
		}
		if !bytes.Equal(item.Value, items[i].Value) {
			t.Fatalf("Unexpected value=[%s] returned. Expected [%s]", item.Value, items[i].Value)
		}
	}
}

func cacher_GetMulti(c Cacher, t *testing.T) {
	itemsCount := 1000
	items := make([]Item, itemsCount)
	for i := 0; i < itemsCount; i++ {
		item := &items[i]
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		if err := c.Set(item); err != nil {
			t.Fatalf("error in client.Set(): [%s]", err)
		}
	}

	checkItems(c, items, t)
}

func TestClient_GetMulti(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_GetMulti(c, t)
}

func cacher_SetNowait(c Cacher, t *testing.T) {
	itemsCount := 1000
	items := make([]Item, itemsCount)
	for i := 0; i < itemsCount; i++ {
		item := &items[i]
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		c.SetNowait(item)
	}

	checkItems(c, items, t)
}

func TestClient_SetNowait(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_SetNowait(c, t)
}

func cacher_CsetNowait(c Cacher, t *testing.T) {
	itemsCount := 1000
	items := make([]Citem, itemsCount)
	for i := 0; i < itemsCount; i++ {
		item := &items[i]
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		item.Etag = uint64(i)
		item.ValidateTtl = time.Second * time.Duration(i)
		c.CsetNowait(item)
	}

	checkCItems(c, items, t)
}

func TestClient_CsetNowait(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_CsetNowait(c, t)
}

func cacher_Delete(c Cacher, t *testing.T) {
	itemsCount := 100
	var item Item
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		if err := c.Delete(item.Key); err != ErrCacheMiss {
			t.Fatalf("error when deleting non-existing item: [%s]", err)
		}
		if err := c.Set(&item); err != nil {
			t.Fatalf("error in client.Set(): [%s]", err)
		}
		if err := c.Delete(item.Key); err != nil {
			t.Fatalf("error when deleting existing item: [%s]", err)
		}
		if err := c.Delete(item.Key); err != ErrCacheMiss {
			t.Fatalf("error when deleting non-existing item: [%s]", err)
		}
	}
}

func TestClient_Delete(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_Delete(c, t)
}

func cacher_DeleteNowait(c Cacher, t *testing.T) {
	itemsCount := 100
	var item Item
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		if err := c.Set(&item); err != nil {
			t.Fatalf("error in client.Set(): [%s]", err)
		}
	}
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		c.DeleteNowait(item.Key)
	}
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		if err := c.Get(&item); err != ErrCacheMiss {
			t.Fatalf("error when obtaining deleted item for key=[%s]: [%s]", item.Key, err)
		}
	}
}

func TestClient_DeleteNowait(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_DeleteNowait(c, t)
}

func cacher_FlushAll(c Cacher, t *testing.T) {
	itemsCount := 100
	var item Item
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		if err := c.Set(&item); err != nil {
			t.Fatalf("error in client.Set(): [%s]", err)
		}
	}
	c.FlushAllNowait()
	c.FlushAll()
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		if err := c.Get(&item); err != ErrCacheMiss {
			t.Fatalf("error when obtaining deleted item: [%s]", err)
		}
	}
}

func TestClient_FlushAll(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_FlushAll(c, t)
}

func cacher_FlushAllDelayed(c Cacher, t *testing.T) {
	itemsCount := 100
	var item Item
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		item.Value = []byte(fmt.Sprintf("value_%d", i))
		if err := c.Set(&item); err != nil {
			t.Fatalf("error in client.Set(): [%s]", err)
		}
	}
	c.FlushAllDelayedNowait(time.Second)
	c.FlushAllDelayed(time.Second)
	foundItems := 0
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		err := c.Get(&item)
		if err == ErrCacheMiss {
			continue
		}
		if err != nil {
			t.Fatalf("error when obtaining item: [%s]", err)
		}
		foundItems++
	}
	if foundItems == 0 {
		t.Fatalf("It seems all the %d items are already delayed", itemsCount)
	}

	time.Sleep(time.Second * 2)
	for i := 0; i < itemsCount; i++ {
		item.Key = []byte(fmt.Sprintf("key_%d", i))
		if err := c.Get(&item); err != ErrCacheMiss {
			t.Fatalf("error when obtaining deleted item: [%s]", err)
		}
	}
}

func TestClient_FlushAllDelayed(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_FlushAllDelayed(c, t)
}

func checkMalformedKey(c Cacher, key string, t *testing.T) {
	item := Item{
		Key: []byte(key),
	}
	if err := c.Get(&item); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}
	if err := c.GetDe(&item, time.Second); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}
	if err := c.Set(&item); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}
	if err := c.Delete(item.Key); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}

	citem := Citem{
		Key: item.Key,
	}
	if err := c.Cget(&citem); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}
	if err := c.Cset(&citem); err != ErrMalformedKey {
		t.Fatalf("Unexpected err=[%s] returned. Expected ErrMalformedKey", err)
	}
}

func cacher_MalformedKey(c Cacher, t *testing.T) {
	checkMalformedKey(c, "malformed key with spaces", t)
	checkMalformedKey(c, "malformed\nkey\nwith\nnewlines", t)
}

func TestClient_MalformedKey(t *testing.T) {
	c, s, cache := newClientServerCache(t)
	defer cache.Close()
	defer s.Stop()
	c.Start()
	defer c.Stop()

	cacher_MalformedKey(c, t)
}

func TestDistributedClient_NoServers(t *testing.T) {
	c := DistributedClient{}
	c.Start()
	defer c.Stop()

	item := Item{
		Key:   []byte("key"),
		Value: []byte("value"),
	}
	citem := Citem{
		Key:         item.Key,
		Value:       item.Value,
		Etag:        12345,
		Expiration:  time.Second,
		ValidateTtl: time.Second,
	}
	if err := c.Get(&item); err != ErrNoServers {
		t.Fatalf("Get() should return ErrNoServers, but returned [%s]", err)
	}
	if _, err := c.GetMulti([][]byte{item.Key}); err != ErrNoServers {
		t.Fatalf("GetMulti() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.GetDe(&item, time.Second); err != ErrNoServers {
		t.Fatalf("GetDe() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.Cget(&citem); err != ErrNoServers {
		t.Fatalf("Cget() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.Set(&item); err != ErrNoServers {
		t.Fatalf("Set() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.Cset(&citem); err != ErrNoServers {
		t.Fatalf("Cset() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.Delete(item.Key); err != ErrNoServers {
		t.Fatalf("Delete() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.FlushAll(); err != ErrNoServers {
		t.Fatalf("FlushAll() should return ErrNoServers, but returned [%s]", err)
	}
	if err := c.FlushAllDelayed(time.Second); err != ErrNoServers {
		t.Fatalf("FlushAllDelayed() should return ErrNoServers, but returned [%s]", err)
	}
}

func closeCaches(caches []*ybc.Cache) {
	for _, cache := range caches {
		cache.Close()
	}
}

func stopServers(servers []*Server) {
	for _, server := range servers {
		server.Stop()
	}
}

func addServersToClient(c *DistributedClient, ss []*Server) {
	for _, s := range ss {
		c.AddServer(s.ListenAddr)
	}
}

func TestDistibutedClient_StartStop(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)

	cacher_StartStop(c)
}

func TestDistibutedClient_AddDeleteServer(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()

	addServersToClient(c, ss)
	for _, s := range ss {
		c.DeleteServer(s.ListenAddr)
	}
}

func TestDistributedClient_StartStop_Multi(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)

	cacher_StartStop_Multi(c)
}

func TestDistributedClient_GetSet(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_GetSet(c, t)
}

func TestDistributedClient_GetDe(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_GetDe(c, t)
}

func TestDistributedClient_CgetCset(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_CgetCset(c, t)
}

func TestDistributedClient_GetMulti(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_GetMulti(c, t)
}

func TestDistributedClient_SetNowait(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_SetNowait(c, t)
}

func TestDistributedClient_CsetNowait(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_CsetNowait(c, t)
}

func TestDistributedClient_Delete(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_Delete(c, t)
}

func TestDistributedClient_DeleteNowait(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_DeleteNowait(c, t)
}

func TestDistributedClient_FlushAll(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_FlushAll(c, t)
}

func TestDistributedClient_FlushAllDelayed(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_FlushAllDelayed(c, t)
}

func TestDistributedClient_MalformedKey(t *testing.T) {
	c, ss, caches := newDistributedClientServersCaches(t)
	defer closeCaches(caches)
	defer stopServers(ss)
	c.Start()
	defer c.Stop()
	addServersToClient(c, ss)

	cacher_MalformedKey(c, t)
}