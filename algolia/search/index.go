package search

import (
	"fmt"
	"net/http"
	"time"

	"github.com/algolia/algoliasearch-client-go/algolia/call"
	"github.com/algolia/algoliasearch-client-go/algolia/iterator"
	"github.com/algolia/algoliasearch-client-go/algolia/opt"
	"github.com/algolia/algoliasearch-client-go/algolia/rand"
	"github.com/algolia/algoliasearch-client-go/algolia/transport"
)

type Index struct {
	appID        string
	name         string
	maxBatchSize int
	transport    *transport.Transport
}

func newIndex(appID, name string, maxBatchSize int, transport *transport.Transport) *Index {
	return &Index{
		appID:        appID,
		name:         name,
		maxBatchSize: maxBatchSize,
		transport:    transport,
	}
}

func (i *Index) path(format string, a ...interface{}) string {
	prefix := fmt.Sprintf("/1/indexes/%s", i.name)
	suffix := fmt.Sprintf(format, a...)
	return prefix + suffix
}

func (i *Index) Clear(opts ...interface{}) (res UpdateTaskRes, err error) {
	path := i.path("/clear")
	err = i.transport.Request(&res, http.MethodPost, path, nil, call.Write, opts...)
	res.wait = i.waitTask
	return
}

func (i *Index) SaveObject(object interface{}, opts ...interface{}) (res SaveObjectRes, err error) {
	path := i.path("")
	err = i.transport.Request(&res, http.MethodPost, path, object, call.Write, opts...)
	res.wait = i.waitTask
	return
}

func (i *Index) SaveObjects(objects interface{}, opts ...interface{}) (res MultipleBatchRes, err error) {
	var (
		object     interface{}
		batch      []interface{}
		operations []Batch
		response   BatchRes
	)

	it := iterator.New(objects)
	autoGenerateObjectIDIfNotExist := opt.ExtractAutoGenerateObjectIDIfNotExist(opts...)

	for {
		object, err = it.Next()
		if err != nil {
			err = fmt.Errorf("iteration failed unexpectedly: %v", err)
			return
		}

		if !autoGenerateObjectIDIfNotExist && object != nil && !hasObjectIDField(object) {
			err = fmt.Errorf("missing objectID in object %#v", object)
			return
		}

		if len(batch) >= i.maxBatchSize || object == nil {
			operations, err = newOperationBatch(batch, AddObject)
			if err != nil {
				err = fmt.Errorf("could not generate intermediate batch: %v", err)
				return
			}
			response, err = i.Batch(operations, opts...)
			if err != nil {
				err = fmt.Errorf("could not send intermediate batch: %v", err)
				return
			}
			res.responses = append(res.responses, response)
		} else {
			batch = append(batch, object)
		}

		if object == nil {
			break
		}
	}

	return
}

func (i *Index) Batch(operations []Batch, opts ...interface{}) (res BatchRes, err error) {
	path := i.path("/batch")
	body := map[string][]Batch{
		"requests": operations,
	}
	err = i.transport.Request(&res, http.MethodPost, path, body, call.Write, opts...)
	res.wait = i.waitTask
	return
}

func (i *Index) GetStatus(taskID int) (res TaskStatusRes, err error) {
	path := i.path("/task/%d", taskID)
	err = i.transport.Request(&res, http.MethodGet, path, nil, call.Read)
	return
}

func (i *Index) waitTask(taskID int) error {
	var maxDuration = time.Second

	for {
		res, err := i.GetStatus(taskID)
		if err != nil {
			return err
		}

		if res.Status == "published" {
			return nil
		}

		sleepDuration := rand.Duration(maxDuration)
		time.Sleep(sleepDuration)

		// Increase the upper boundary used to generate the sleep duration
		if maxDuration < 10*time.Minute {
			maxDuration *= 2
			if maxDuration > 10*time.Minute {
				maxDuration = 10 * time.Minute
			}
		}
	}
}
