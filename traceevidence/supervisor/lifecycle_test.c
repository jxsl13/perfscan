/* Source-level unit harness: no real spawn, wait, kill or Darwin notification.
 * Fake PID values can NEVER reach an OS process-control call. Real operations
 * are limited to this harness's private files/pipes and SIGCHLD disposition. */
#define main supervisor_main
#define posix_spawn test_spawn
#define waitpid test_waitpid
#define kill test_kill
#define notify_register_check test_register
#define notify_check test_notify_check
#define notify_cancel test_notify_cancel
#include "supervisor.c"
#undef main
#undef posix_spawn
#undef waitpid
#undef kill
#undef notify_register_check
#undef notify_check
#undef notify_cancel
static int dead_recorder, dead_target, resumes, killed, notifications, bad_sigchld;
static const pid_t target_id=111111, recorder_id=222222;
int test_spawn(pid_t *p,const char *path,const posix_spawn_file_actions_t *a,const posix_spawnattr_t *attr,char *const args[],char *const env[]) {
  (void)path;(void)a;(void)args;(void)env;
  struct sigaction observed;
  if(sigaction(SIGCHLD,NULL,&observed)) return EINVAL;
  if(observed.sa_handler!=SIG_DFL || (observed.sa_flags&SA_NOCLDWAIT)) bad_sigchld=1;
  *p=attr?target_id:recorder_id;
  return 0;
}
pid_t test_waitpid(pid_t p,int *status,int options) {
  if(options!=WNOHANG) { errno=EINVAL;return -1; }
  if(p==recorder_id&&(dead_recorder||resumes||killed)) { *status=killed?SIGTERM:0;return p; }
  if(p==target_id&&(dead_target||resumes||killed)) { *status=killed?SIGTERM:0;return p; }
  if(p!=target_id&&p!=recorder_id) { errno=ECHILD;return -1; }
  return 0;
}
int test_kill(pid_t p,int sig) {
  if(p!=target_id&&p!=recorder_id) {errno=ESRCH;return -1;}
  if(sig==SIGCONT&&p==target_id) resumes++;
  else if(sig==SIGTERM||sig==SIGKILL) killed++;
  else {errno=EINVAL;return -1;}
  return 0;
}
uint32_t test_register(const char *name,int *token) { (void)name;*token=1;return NOTIFY_STATUS_OK; }
uint32_t test_notify_check(int token,int *state) { (void)token;*state=notifications++>0;return NOTIFY_STATUS_OK; }
uint32_t test_notify_cancel(int token) { (void)token;return NOTIFY_STATUS_OK; }
int main(int argc,char **argv) {
  if(argc!=3)return 2;
  dead_recorder=!strcmp(argv[2],"dead-recorder");
  dead_target=!strcmp(argv[2],"dead-target");
  /* Prove supervisor resets an inherited ignored/no-zombie disposition. */
  struct sigaction ignored={0};ignored.sa_handler=SIG_IGN;ignored.sa_flags=SA_NOCLDWAIT;
  if(sigemptyset(&ignored.sa_mask)||sigaction(SIGCHLD,&ignored,NULL))return 3;
  int control_pipe[2];if(pipe(control_pipe))return 3;
  if(dup2(control_pipe[0],STDIN_FILENO)<0||close(control_pipe[0]))return 3;
  char *args[]={"supervisor",argv[1],"","100","500","100","4096","/never/executed/xcrun","100ms","/never/executed/target",NULL};
  int result=supervisor_main(10,args);
  if(close(control_pipe[1]))return 3;
  if(bad_sigchld)return 4;
  if(dead_recorder) {if(resumes||result!=1||notifications!=1)return 5;}
  else if(dead_target) {if(resumes||result!=1)return 7;}
  else if(resumes!=1||result!=0)return 6;
  return 0;
}
