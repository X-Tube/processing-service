package video

type UploadInput struct {
	VideoID string
	Bucket  string
	Key     string
}

type Profile struct {
	Name   string
	Height int
}

type Segment struct {
	Profile  string
	Index    int
	FilePath string
	FileName string
	Size     int64
}

type ProcessorConfig struct {
	Name         string
	InputBucket  string
	OutputBucket string
	TempDir      string
	Profiles     []Profile
	ChunkDetails bool
}
