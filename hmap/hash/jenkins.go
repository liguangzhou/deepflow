package hash

// Jenkins Wiki： https://en.wikipedia.org/wiki/Jenkins_hash_function
// 64位算法： https://blog.csdn.net/yueyedeai/article/details/17025265
// 32位算法： http://burtleburtle.net/bob/hash/integer.html

// Jenkins哈希的两个关键特性是：
//   1.雪崩性（更改输入参数的任何一位，就将引起输出有一半以上的位发生变化）
//   2.可逆性
// 目前我们仅用到雪崩性，来获得更好的分布

func Jenkins(hash uint64) int32 {
	hash = (hash << 21) - hash - 1
	hash = hash ^ (hash >> 24)
	hash = (hash + (hash << 3)) + (hash << 8) // hash * 265
	hash = hash ^ (hash >> 14)
	hash = (hash + (hash << 2)) + (hash << 4) // hash * 21
	hash = hash ^ (hash >> 28)
	hash = hash + (hash << 31)
	return int32((hash >> 32) ^ hash)
}

func Jenkins32(hash uint32) int32 {
	hash = (hash << 11) - hash - 1
	hash = hash ^ (hash >> 12)
	hash = (hash + (hash << 3)) + (hash << 8) // hash * 265
	hash = hash ^ (hash >> 7)
	hash = (hash + (hash << 2)) + (hash << 4) // hash * 21
	hash = hash ^ (hash >> 14)
	hash = hash + (hash << 16)
	return int32(hash)
}
