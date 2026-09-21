package setting

// StreamCacheQueueLength controls the optional stream-mode response cache.
// Sensitive-word policy is stored in the sensitive_word_policy table and is
// no longer exposed through setting globals.
var StreamCacheQueueLength = 0
