package filewal

import "os"

func WriteAndReadBack(path string, data []byte) ([]byte, error) {
	w, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Sync(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	buf := make([]byte, len(data))
	n, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func WriteReadFileRoundTrip(path string, data []byte) ([]byte, error) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func SizeOf(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func MoveFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func DeleteThenCheckGone(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		return false, err
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err), nil
}
