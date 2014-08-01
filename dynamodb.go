package timedb

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"encoding/base64"
	"fmt"
	"github.com/FlukeNetworks/timedb/bucket"
	"github.com/crowdmob/goamz/dynamodb"
	"strconv"
	"time"
)

type DynamoDBStore struct {
	BucketStore
	repo DynamoDBRepository
}

func NewDynamoDBStore(store BucketStore, table *dynamodb.Table, multiplier float64) *DynamoDBStore {
	ret := &DynamoDBStore{
		store,
		DynamoDBRepository{
			Multiplier:  multiplier,
			Granularity: store.Granularity,
			Table:       table,
		},
	}
	ret.Repository = ret.repo
	return ret
}

type DynamoDBRepository struct {
	Multiplier  float64
	Granularity time.Duration
	Table       *dynamodb.Table
}

func (self DynamoDBRepository) Put(series uuid.UUID, granularity time.Duration, start time.Time, attributes []EncodedBucketAttribute) error {
	hashKey := fmt.Sprintf("%s|%d", series.String(), int64(granularity.Seconds()))
	rangeKey := fmt.Sprintf("%d", start.Unix())
	bAttribs := make([]dynamodb.Attribute, len(attributes))
	for i, encodedAttribute := range attributes {
		bAttribs[i] = dynamodb.Attribute{
			Type:  dynamodb.TYPE_BINARY,
			Name:  encodedAttribute.Name,
			Value: base64.StdEncoding.EncodeToString(encodedAttribute.Data),
		}
	}
	_, err := self.Table.PutItem(hashKey, rangeKey, bAttribs)
	return err
}

func (self DynamoDBRepository) entryReader(series uuid.UUID, item map[string]*dynamodb.Attribute, attributes []string) (EntryReader, error) {
	tData, err := base64.StdEncoding.DecodeString(item[TimeAttribute].Value)
	if err != nil {
		return nil, err
	}
	startUnix, err := strconv.ParseInt(item["time"].Value, 10, 64)
	if err != nil {
		return nil, err
	}
	decs := map[string]*bucket.BucketDecoder{
		TimeAttribute: bucket.NewBucketDecoder(startUnix, bytes.NewBuffer(tData)),
	}
	for _, a := range attributes {
		data, err := base64.StdEncoding.DecodeString(item[a].Value)
		if err != nil {
			return nil, err
		}
		decs[a] = bucket.NewBucketDecoder(0, bytes.NewBuffer(data))
	}
	return bucketEntryReader(series, self.Multiplier, decs, attributes), nil
}

func (self DynamoDBRepository) Query(series uuid.UUID, start, end time.Time, attributes []string, entries chan Entry, errors chan error) {
	comparisons := []dynamodb.AttributeComparison{
		*dynamodb.NewEqualStringAttributeComparison("series", fmt.Sprintf("%s|%d", series.String(), int64(self.Granularity.Seconds()))),
		*dynamodb.NewNumericAttributeComparison("time", dynamodb.COMPARISON_GREATER_THAN_OR_EQUAL, start.Unix()),
		*dynamodb.NewNumericAttributeComparison("time", dynamodb.COMPARISON_LESS_THAN, end.Unix()),
	}
	items, err := self.Table.Query(comparisons)
	if err != nil {
		errors <- err
		return
	}
	for _, item := range items {
		reader, err := self.entryReader(series, item, attributes)
		if err != nil {
			errors <- err
			return
		}
		entryBuf := make([]Entry, 1)
		for i, _ := range entryBuf {
			entryBuf[i].Attributes = map[string]float64{}
		}
		for {
			n, err := reader.ReadEntries(entryBuf)
			if n > 0 {
				for _, e := range entryBuf[:n] {
					fmt.Printf("%v\n", e)
					entries <- e
				}
			}
			if err != nil {
				break
			}
		}
	}
}

type DynamoDBCache struct {
	Table *dynamodb.Table
}

func (self *DynamoDBCache) Query(series uuid.UUID, start, end time.Time, attributes []string, entries chan Entry, errors chan error) {
	conditions := []dynamodb.AttributeComparison{
		*dynamodb.NewEqualStringAttributeComparison("series", series.String()),
		*dynamodb.NewNumericAttributeComparison("time", dynamodb.COMPARISON_GREATER_THAN_OR_EQUAL, start.Unix()),
		*dynamodb.NewNumericAttributeComparison("time", dynamodb.COMPARISON_LESS_THAN_OR_EQUAL, end.Unix()),
	}
	items, err := self.Table.Query(conditions)
	if err != nil {
		errors <- err
		return
	}
	for _, item := range items {
		e := Entry{
			Attributes: map[string]float64{},
		}
		for name, a := range item {
			if name == "series" {
				continue
			}
			if name == "time" {
				unixTime, err := strconv.ParseInt(a.Value, 10, 64)
				if err != nil {
					errors <- err
					continue
				}
				e.Timestamp = time.Unix(unixTime, 0)
			} else {
				value, err := strconv.ParseFloat(a.Value, 64)
				if err != nil {
					errors <- err
					continue
				}
				e.Attributes[name] = value
			}
		}
		entries <- e
	}
}

func (self *DynamoDBCache) Insert(series uuid.UUID, entry Entry) error {
	attribs := []dynamodb.Attribute{
		dynamodb.Attribute{
			Type:  "N",
			Name:  "raw",
			Value: fmt.Sprintf("%v", entry.Attributes["raw"]),
		},
	}
	_, err := self.Table.PutItem(series.String(), fmt.Sprintf("%v", entry.Timestamp.Unix()), attribs)
	return err
}
