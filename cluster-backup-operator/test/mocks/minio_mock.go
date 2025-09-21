package mocks

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// MockMinioClient provides a mock implementation of Minio client for testing
type MockMinioClient struct {
	objects map[string][]byte
	mutex   sync.RWMutex

	// Configuration for simulating failures
	ShouldFailPutObject   bool
	ShouldFailGetObject   bool
	ShouldFailListObjects bool

	// Tracking for test verification
	PutObjectCalls   []PutObjectCall
	GetObjectCalls   []GetObjectCall
	ListObjectsCalls []ListObjectsCall
}

// PutObjectCall represents a call to PutObject for test verification
type PutObjectCall struct {
	Bucket string
	Key    string
	Size   int64
	Time   time.Time
}

// GetObjectCall represents a call to GetObject for test verification
type GetObjectCall struct {
	Bucket string
	Key    string
	Time   time.Time
}

// ListObjectsCall represents a call to ListObjects for test verification
type ListObjectsCall struct {
	Bucket string
	Prefix string
	Time   time.Time
}

// ObjectInfo represents information about a stored object
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// NewMockMinioClient creates a new mock Minio client
func NewMockMinioClient() *MockMinioClient {
	return &MockMinioClient{
		objects:          make(map[string][]byte),
		PutObjectCalls:   make([]PutObjectCall, 0),
		GetObjectCalls:   make([]GetObjectCall, 0),
		ListObjectsCalls: make([]ListObjectsCall, 0),
	}
}

// PutObject stores an object in the mock storage
func (m *MockMinioClient) PutObject(bucket, key string, reader io.Reader, size int64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Record the call for test verification
	m.PutObjectCalls = append(m.PutObjectCalls, PutObjectCall{
		Bucket: bucket,
		Key:    key,
		Size:   size,
		Time:   time.Now(),
	})

	if m.ShouldFailPutObject {
		return fmt.Errorf("mock error: failed to put object %s/%s", bucket, key)
	}

	// Read the data from the reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Store the object
	objectKey := fmt.Sprintf("%s/%s", bucket, key)
	m.objects[objectKey] = data

	return nil
}

// GetObject retrieves an object from the mock storage
func (m *MockMinioClient) GetObject(bucket, key string) (io.Reader, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Record the call for test verification
	m.GetObjectCalls = append(m.GetObjectCalls, GetObjectCall{
		Bucket: bucket,
		Key:    key,
		Time:   time.Now(),
	})

	if m.ShouldFailGetObject {
		return nil, fmt.Errorf("mock error: failed to get object %s/%s", bucket, key)
	}

	objectKey := fmt.Sprintf("%s/%s", bucket, key)
	data, exists := m.objects[objectKey]
	if !exists {
		return nil, fmt.Errorf("object not found: %s/%s", bucket, key)
	}

	return bytes.NewReader(data), nil
}

// ListObjects lists objects in the mock storage with optional prefix
func (m *MockMinioClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Record the call for test verification
	m.ListObjectsCalls = append(m.ListObjectsCalls, ListObjectsCall{
		Bucket: bucket,
		Prefix: prefix,
		Time:   time.Now(),
	})

	if m.ShouldFailListObjects {
		return nil, fmt.Errorf("mock error: failed to list objects in bucket %s", bucket)
	}

	var objects []ObjectInfo
	bucketPrefix := fmt.Sprintf("%s/", bucket)

	for objectKey, data := range m.objects {
		if !strings.HasPrefix(objectKey, bucketPrefix) {
			continue
		}

		// Extract the key without bucket prefix
		key := strings.TrimPrefix(objectKey, bucketPrefix)

		// Apply prefix filter if specified
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}

		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         int64(len(data)),
			LastModified: time.Now(), // Mock timestamp
		})
	}

	return objects, nil
}

// DeleteObject removes an object from the mock storage
func (m *MockMinioClient) DeleteObject(bucket, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	objectKey := fmt.Sprintf("%s/%s", bucket, key)
	delete(m.objects, objectKey)

	return nil
}

// BucketExists checks if a bucket exists (always returns true in mock)
func (m *MockMinioClient) BucketExists(bucket string) bool {
	return true
}

// CreateBucket creates a bucket (no-op in mock)
func (m *MockMinioClient) CreateBucket(bucket string) error {
	return nil
}

// Reset clears all stored objects and call history
func (m *MockMinioClient) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.objects = make(map[string][]byte)
	m.PutObjectCalls = make([]PutObjectCall, 0)
	m.GetObjectCalls = make([]GetObjectCall, 0)
	m.ListObjectsCalls = make([]ListObjectsCall, 0)

	m.ShouldFailPutObject = false
	m.ShouldFailGetObject = false
	m.ShouldFailListObjects = false
}

// GetStoredObjectCount returns the number of stored objects
func (m *MockMinioClient) GetStoredObjectCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.objects)
}

// GetStoredObject returns the raw data of a stored object
func (m *MockMinioClient) GetStoredObject(bucket, key string) ([]byte, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	objectKey := fmt.Sprintf("%s/%s", bucket, key)
	data, exists := m.objects[objectKey]
	return data, exists
}

// HasObject checks if an object exists in storage
func (m *MockMinioClient) HasObject(bucket, key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	objectKey := fmt.Sprintf("%s/%s", bucket, key)
	_, exists := m.objects[objectKey]
	return exists
}

// GetPutObjectCallCount returns the number of PutObject calls made
func (m *MockMinioClient) GetPutObjectCallCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.PutObjectCalls)
}

// GetGetObjectCallCount returns the number of GetObject calls made
func (m *MockMinioClient) GetGetObjectCallCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.GetObjectCalls)
}

// GetLastPutObjectCall returns the most recent PutObject call
func (m *MockMinioClient) GetLastPutObjectCall() *PutObjectCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(m.PutObjectCalls) == 0 {
		return nil
	}

	return &m.PutObjectCalls[len(m.PutObjectCalls)-1]
}

// GetLastGetObjectCall returns the most recent GetObject call
func (m *MockMinioClient) GetLastGetObjectCall() *GetObjectCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(m.GetObjectCalls) == 0 {
		return nil
	}

	return &m.GetObjectCalls[len(m.GetObjectCalls)-1]
}

// SimulateNetworkError configures the mock to simulate network failures
func (m *MockMinioClient) SimulateNetworkError() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ShouldFailPutObject = true
	m.ShouldFailGetObject = true
	m.ShouldFailListObjects = true
}

// SimulatePutObjectError configures the mock to fail PutObject calls
func (m *MockMinioClient) SimulatePutObjectError() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ShouldFailPutObject = true
}

// SimulateGetObjectError configures the mock to fail GetObject calls
func (m *MockMinioClient) SimulateGetObjectError() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ShouldFailGetObject = true
}

// StoreTestBackup stores a test backup in the mock storage
func (m *MockMinioClient) StoreTestBackup(bucket, backupName string, resources map[string][]byte) error {
	for resourceType, data := range resources {
		key := fmt.Sprintf("backups/%s/%s.yaml", backupName, resourceType)
		reader := bytes.NewReader(data)

		err := m.PutObject(bucket, key, reader, int64(len(data)))
		if err != nil {
			return err
		}
	}

	return nil
}

// GetTestBackup retrieves a test backup from the mock storage
func (m *MockMinioClient) GetTestBackup(bucket, backupName string) (map[string][]byte, error) {
	prefix := fmt.Sprintf("backups/%s/", backupName)
	objects, err := m.ListObjects(bucket, prefix)
	if err != nil {
		return nil, err
	}

	resources := make(map[string][]byte)

	for _, obj := range objects {
		reader, err := m.GetObject(bucket, obj.Key)
		if err != nil {
			return nil, err
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}

		// Extract resource type from key
		resourceType := strings.TrimSuffix(strings.TrimPrefix(obj.Key, prefix), ".yaml")
		resources[resourceType] = data
	}

	return resources, nil
}
