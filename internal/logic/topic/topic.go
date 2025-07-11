package topic

import (
	"errors"
	"log"

	"github.com/jekiapp/topic-master/internal/model/entity"
	dbPkg "github.com/jekiapp/topic-master/pkg/db"
	"github.com/tidwall/buntdb"
)

type ISyncTopics interface {
	GetAllTopics() ([]string, error)
	GetAllNsqTopicEntities() ([]entity.Entity, error)
	CreateNsqTopicEntity(topic string) (*entity.Entity, error)
	DeleteNsqTopicEntity(topic string) error
}

func SyncTopics(db *buntdb.DB, iSyncTopics ISyncTopics) (topics []string, err error) {
	// Get the list of topics from the source (e.g., nsqlookupd)
	topics, err = iSyncTopics.GetAllTopics()
	if err != nil {
		return nil, err
	}

	if len(topics) == 0 {
		log.Println("[WARN] No topics found")
	}
	// Build a set for fast lookup of valid topics
	topicSet := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		topicSet[t] = struct{}{}
	}

	// Get all topic entities currently in the DB
	dbEntities, err := iSyncTopics.GetAllNsqTopicEntities()
	if err != nil && err != dbPkg.ErrNotFound {
		return nil, err
	}

	var errSet error
	// Build a set for fast lookup of DB topics
	dbTopicSet := make(map[string]struct{}, len(dbEntities))
	for _, entity := range dbEntities {
		dbTopicSet[entity.Name] = struct{}{}
		// If a topic exists in DB but not in the source, delete it from DB
		// another option is to mark it as deleted
		if _, ok := topicSet[entity.Name]; !ok {
			log.Println("[INFO] Deleting topic from DB: ", entity.Name)
			if delErr := iSyncTopics.DeleteNsqTopicEntity(entity.Name); delErr != nil {
				// Collect deletion errors
				errSet = errors.Join(errSet, errors.New("DeleteNsqTopicEntity("+entity.Name+"): "+delErr.Error()))
			}
		}
	}

	// For each topic in the source, if not found in DB, create it in DB
	for t := range topicSet {
		if _, ok := dbTopicSet[t]; !ok {
			log.Println("[INFO] Creating topic in DB: ", t)
			if _, createErr := iSyncTopics.CreateNsqTopicEntity(t); createErr != nil {
				// Collect creation errors
				errSet = errors.Join(errSet, errors.New("CreateNsqTopicEntity("+t+"): "+createErr.Error()))
			}
		}
	}

	// Return any collected errors (nil if none)
	return topics, errSet
}
