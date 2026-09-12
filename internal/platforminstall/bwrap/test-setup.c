#include <stdarg.h>
#include <sys/prctl.h>
#include <unistd.h>
static long test_syscall (long number, ...);
static int test_prctl (int option, ...);
#define syscall test_syscall
#define prctl test_prctl
#define main bubblewrap_program_main
#include "bubblewrap.c"
#undef main
#undef syscall
#undef prctl

static const char *failure = "none";
static int rule_count;
static int ruleset_descriptor = -1;
static int rule_descriptors[8];
static bool enforced;

static int
test_prctl (int option, ...)
{
  if (option == PR_GET_NO_NEW_PRIVS)
    return strcmp (failure, "nnp") == 0 ? 0 : 1;
  abort ();
}

static long
test_syscall (long number, ...)
{
  va_list args;
  va_start (args, number);
  if (number == __NR_landlock_create_ruleset)
    {
      struct landlock_ruleset_attr *attr = va_arg (args, struct landlock_ruleset_attr *);
      size_t size = va_arg (args, size_t);
      int flags = va_arg (args, int);
      va_end (args);
      if (attr == NULL)
        {
          assert (size == 0 && flags == LANDLOCK_CREATE_RULESET_VERSION);
          return strcmp (failure, "abi") == 0 ? 2 : 3;
        }
      assert (attr->handled_access_fs == (LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_TRUNCATE | LANDLOCK_ACCESS_FS_MAKE_CHAR | LANDLOCK_ACCESS_FS_MAKE_BLOCK));
      if (strcmp (failure, "create") == 0)
        { errno = EPERM; return -1; }
      ruleset_descriptor = open ("/dev/null", O_RDONLY | O_CLOEXEC);
      return ruleset_descriptor;
    }
  if (number == __NR_landlock_add_rule)
    {
      int fd = va_arg (args, int);
      int kind = va_arg (args, int);
      struct landlock_path_beneath_attr *rule = va_arg (args, struct landlock_path_beneath_attr *);
      int flags = va_arg (args, int);
      assert (fd == ruleset_descriptor && kind == LANDLOCK_RULE_PATH_BENEATH && flags == 0);
      assert (rule->allowed_access == (LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_TRUNCATE));
      assert (rule_count < 8);
      rule_descriptors[rule_count++] = rule->parent_fd;
      va_end (args);
      if (strcmp (failure, "add") == 0)
        { errno = EPERM; return -1; }
      return 0;
    }
  if (number == __NR_landlock_restrict_self)
    {
      int fd = va_arg (args, int);
      int flags = va_arg (args, int);
      assert (fd == ruleset_descriptor && flags == 0);
      va_end (args);
      if (strcmp (failure, "restrict") == 0)
        { errno = EPERM; return -1; }
      enforced = TRUE;
      return 0;
    }
  abort ();
}

int
main (int argc, char **argv)
{
  if (argc == 2 && strcmp (argv[1], "identity") == 0)
    {
      struct stat st = { .st_mode = S_IFCHR | 0666, .st_rdev = makedev (1, 9),
                         .st_uid = 0, .st_gid = 0, .st_nlink = 1 };
      assert (entropy_identity_matches (&st, TRUE, 1000, 1000));
      st.st_uid = st.st_gid = 65534;
      assert (!entropy_identity_matches (&st, TRUE, 1000, 1000));
      assert (entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_uid = st.st_gid = 65533;
      assert (entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_uid = 1000;
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_uid = 65534; st.st_gid = 1000;
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_uid = st.st_gid = 65534;
      st.st_mode = S_IFREG | 0666;
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_mode = S_IFCHR | 0666; st.st_rdev = makedev (1, 8);
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_rdev = makedev (1, 9); st.st_mode = S_IFCHR | 0600;
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      st.st_mode = S_IFCHR | 0666; st.st_nlink = 2;
      assert (!entropy_identity_matches (&st, FALSE, 1000, 1000));
      return 0;
    }
  if (argc == 2 && strcmp (argv[1], "host-identity") == 0)
    {
      real_uid = getuid (); real_gid = getgid (); opt_readonly_entropy = TRUE;
      verify_host_entropy ();
      return 0;
    }
  if (argc == 4 && strcmp (argv[1], "policy") == 0)
    {
      failure = argv[2];
      opt_readonly_entropy = TRUE;
      SetupOp *op = setup_op_new (SETUP_BIND_MOUNT);
      op->source = argv[3];
      op->dest = argv[3];
      if (strcmp (failure, "conditional") == 0)
        op->flags = ALLOW_NOTEXIST;
      if (strcmp (failure, "unexpected") == 0)
        op->type = SETUP_MOUNT_TMPFS;
      restrict_entropy_writes ();
      assert (enforced && rule_count == 2);
      assert (fcntl (ruleset_descriptor, F_GETFD) == -1 && errno == EBADF);
      for (int i = 0; i < rule_count; i++)
        assert (fcntl (rule_descriptors[i], F_GETFD) == -1 && errno == EBADF);
      return 0;
    }
  const char **args = (const char **) argv + 1;
  int count = argc - 1;
  parse_args (&count, &args);
  assert (ops != NULL && ops->next == NULL && count == 1 && strcmp (args[0], "/fixture-command") == 0);
  if (ops->type == SETUP_RO_BIND_NULL)
    assert (!opt_readonly_entropy && strcmp (ops->source, "/dev/null") == 0 && strcmp (ops->dest, "/dev/null") == 0);
  else
    assert (ops->type == SETUP_RO_BIND_ENTROPY && opt_readonly_entropy && strcmp (ops->source, "/dev/urandom") == 0 && strcmp (ops->dest, "/dev/urandom") == 0);
  return 0;
}
