// The last error of each thread. The core may be called from several
// threads (imports in the background, the reader on the main thread), so
// one thread's failure must not overwrite what another is about to read.

#include <stdlib.h>
#include <string.h>

static _Thread_local char *last_error;
static _Thread_local int last_error_kind;

// molecore_set_error replaces the calling thread's last error with a copy
// of msg, and its kind with kind.
void molecore_set_error(const char *msg, int kind) {
    free(last_error);
    last_error = strdup(msg);
    last_error_kind = kind;
}

// molecore_error_kind returns the kind of the calling thread's last error.
int molecore_error_kind(void) {
    return last_error_kind;
}

// molecore_copy_error returns a copy of the calling thread's last error, or
// of "" if there is none, for the caller to free.
char *molecore_copy_error(void) {
    return strdup(last_error != NULL ? last_error : "");
}
