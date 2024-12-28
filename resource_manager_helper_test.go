package depman

type TestResourceManager interface {
	ResourceManager

	SetTestLock1(lock chan struct{})
	SetTestLock2(lock chan struct{})
	SetTestLock3(lock chan struct{})
}

var _ TestResourceManager = (*managerImpl)(nil)

func (m *managerImpl) SetTestLock1(lock chan struct{}) {
	m.m.Lock()
	defer m.m.Unlock()

	m.testLock1 = lock
}

func (m *managerImpl) SetTestLock2(lock chan struct{}) {
	m.m.Lock()
	defer m.m.Unlock()

	m.testLock2 = lock
}

func (m *managerImpl) SetTestLock3(lock chan struct{}) {
	m.m.Lock()
	defer m.m.Unlock()

	m.testLock3 = lock
}
