// The last error of each thread. The core may be called from several
// threads (imports in the background, the reader on the main thread), so
// one thread's failure must not overwrite what another is about to read.

#include <stdlib.h>
#include <string.h>

static _Thread_local char *last_error;

// molecore_set_error replaces the calling thread's last error with a copy
// of msg.
void molecore_set_error(const char *msg) {
    free(last_error);
    last_error = strdup(msg);
}

// molecore_copy_error returns a copy of the calling thread's last error, or
// of "" if there is none, for the caller to free.
char *molecore_copy_error(void) {
    return strdup(last_error != NULL ? last_error : "");
}
