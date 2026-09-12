/* Embedded, locally compiled Darwin-only helper. No pid/group/name attachment
 * supplied by caller: attach PID comes exclusively from suspended posix_spawn.
 * One thread owns all waitpid and kill operations. Reaped IDs are cleared before
 * any further operation. stdin EOF/catchable signals enter bounded cleanup.
 * No claim covers descendants, services, supervisor SIGKILL or kernel stalls. */
#include <spawn.h>
#include <notify.h>
#include <sys/wait.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>
#include <time.h>
#include <errno.h>
#include <string.h>
#include <limits.h>
#include <float.h>
extern char **environ;
static volatile sig_atomic_t interrupted;
static void interrupt_handler(int s) { (void)s; interrupted=1; }
static int failed;
static double clock_ms(void) { struct timespec t; if(clock_gettime(CLOCK_MONOTONIC,&t)) { failed=1; return DBL_MAX; } return t.tv_sec*1000.0+t.tv_nsec/1e6; }
struct child { pid_t pid; int status, reaped, forced; };
struct stream { int readfd, writefd; uint64_t bytes; };
static int close_checked(int fd) { if(fd<0) return 0; int r=close(fd); if(r) failed=1; return r; }
static void reap(struct child *c) {
  if(!c->pid) return;
  int s; pid_t r=waitpid(c->pid,&s,WNOHANG);
  if(r==c->pid) { c->pid=0; c->status=s; c->reaped=1; }
  else if(r<0 && errno!=EINTR) { failed=1; c->pid=0; /* ownership uncertain: never signal */ }
}
static int signal_owned(struct child *c,int sig) {
  reap(c); if(!c->pid) return 0;
  if(kill(c->pid,sig)) { failed=1; return 0; }
  return 1;
}
static int stream_init(struct stream *s,const char *path,int *childfd) {
  int p[2]; if(pipe(p)) return -1;
  s->readfd=p[0]; *childfd=p[1];
  s->writefd=open(path,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0600);
  if(s->writefd<0 || fcntl(p[0],F_SETFL,O_NONBLOCK)<0) return -1;
  return 0;
}
static void drain(struct stream *s,uint64_t limit) {
  if(s->readfd<0) return;
  char b[8192]; ssize_t n;
  while((n=read(s->readfd,b,sizeof b))>0) {
    int overflow=(uint64_t)n>limit-s->bytes;
    if(overflow) n=(ssize_t)(limit-s->bytes);
    s->bytes+=(uint64_t)n;
    ssize_t off=0;
    while(off<n) { ssize_t w=write(s->writefd,b+off,(size_t)(n-off)); if(w<0 && errno==EINTR) continue; if(w<=0) { failed=1; return; } off+=w; }
    if(overflow) { failed=1; return; }
  }
  if(n==0) { close_checked(s->readfd); s->readfd=-1; }
  else if(errno!=EAGAIN && errno!=EINTR) failed=1;
}
static void tick(struct child *target,struct child *recorder,struct stream *streams,uint64_t limit) {
  reap(target); reap(recorder);
  for(int i=0;i<4;i++) drain(&streams[i],limit);
}
static void cleanup(struct child *c,struct child *other,struct stream *streams,uint64_t limit,double budget) {
  reap(c); if(!c->pid) return;
  if(signal_owned(c,SIGTERM)) c->forced=1;
  double end=clock_ms()+budget/2;
  while(c->pid && clock_ms()<end) { tick(c,other,streams,limit); usleep(10000); }
  if(c->pid) signal_owned(c,SIGKILL);
  end=clock_ms()+budget/2;
  while(c->pid && clock_ms()<end) { tick(c,other,streams,limit); usleep(10000); }
  if(c->pid) failed=1;
}
static int actions(posix_spawn_file_actions_t *a,struct stream *s,int *ends,int first) {
  int e;
  if((e=posix_spawn_file_actions_addopen(a,STDIN_FILENO,"/dev/null",O_RDONLY,0))) return e;
  if((e=posix_spawn_file_actions_adddup2(a,ends[first],STDOUT_FILENO))) return e;
  if((e=posix_spawn_file_actions_adddup2(a,ends[first+1],STDERR_FILENO))) return e;
  for(int i=0;i<4;i++) {
    if((e=posix_spawn_file_actions_addclose(a,s[i].readfd))) return e;
    if((e=posix_spawn_file_actions_addclose(a,s[i].writefd))) return e;
    if((e=posix_spawn_file_actions_addclose(a,ends[i]))) return e;
  }
  return 0;
}
static long number(const char *s,long maximum) { char *end; errno=0; long n=strtol(s,&end,10); return errno||*end||n<1||n>maximum ? -1:n; }
int main(int argc,char **argv) {
  /* root, cwd, readiness-ms, command-ms, cleanup-ms, stream-limit, xcrun,
   * recording-time-limit, workload executable and arguments */
  if(argc<10) return 2;
  long readiness=number(argv[3],86400000), total=number(argv[4],86400000), cleanup_ms=number(argv[5],30000), limit=number(argv[6],67108864);
  if(readiness<0||total<0||cleanup_ms<0||limit<0) return 2;
  struct child target={0}, recorder={0};
  struct stream streams[4]; int ends[4];
  for(int i=0;i<4;i++) { streams[i]=(struct stream){.readfd=-1,.writefd=-1,.bytes=0}; ends[i]=-1; }
  int token=-1,ready=0,resumed=0,canceled=0,target_pid=0,recorder_pid=0;
  const char *files[]={"target.txt","target.stderr","record.stdout","record.stderr"};
  char path[PATH_MAX], trace[PATH_MAX], name[128]={0}, pid[32];
  double start=clock_ms(); if(failed) return 3;
  struct sigaction sa={0}; sa.sa_handler=interrupt_handler;
  if(sigemptyset(&sa.sa_mask)||sigaction(SIGTERM,&sa,NULL)||sigaction(SIGINT,&sa,NULL)||fcntl(STDIN_FILENO,F_SETFL,O_NONBLOCK)<0) { failed=1; goto done; }
  struct sigaction child_action={0}; child_action.sa_handler=SIG_DFL;
  if(sigemptyset(&child_action.sa_mask)||sigaction(SIGCHLD,&child_action,NULL)) { failed=1; goto done; }
  char control; ssize_t control_n=read(STDIN_FILENO,&control,1);
  if(interrupted||control_n==0||control_n>0) { canceled=1; goto done; }
  if(control_n<0&&errno!=EAGAIN&&errno!=EINTR) { failed=1; goto done; }
  if(argv[2][0] && chdir(argv[2])) { failed=1; goto done; }
  if(snprintf(trace,sizeof trace,"%s/capture.trace",argv[1])>=(int)sizeof trace || snprintf(name,sizeof name,"org.perfscan.supervisor.%d.%llu",getpid(),(unsigned long long)(start*1000000))>=(int)sizeof name) { failed=1; goto done; }
  if(notify_register_check(name,&token)!=NOTIFY_STATUS_OK) { failed=1; goto done; }
  int state;
  if(notify_check(token,&state)!=NOTIFY_STATUS_OK) { failed=1; goto done; }
  for(int i=0;i<4;i++) {
    if(snprintf(path,sizeof path,"%s/%s",argv[1],files[i])>=(int)sizeof path || stream_init(&streams[i],path,&ends[i])) { failed=1; goto done; }
  }
  control_n=read(STDIN_FILENO,&control,1);
  if(interrupted||control_n==0||control_n>0) { canceled=1; goto done; }
  if(control_n<0&&errno!=EAGAIN&&errno!=EINTR) { failed=1; goto done; }
  posix_spawnattr_t attr; posix_spawn_file_actions_t a;
  if(posix_spawnattr_init(&attr)) { failed=1; goto done; }
  if(posix_spawn_file_actions_init(&a)) { if(posix_spawnattr_destroy(&attr)) failed=1; failed=1; goto done; }
  int e=posix_spawnattr_setflags(&attr,POSIX_SPAWN_START_SUSPENDED);
  int ae=actions(&a,streams,ends,0);
  if(!e&&!ae) e=posix_spawn(&target.pid,argv[9],&a,&attr,&argv[9],environ);
  if(posix_spawn_file_actions_destroy(&a)) failed=1;
  if(posix_spawnattr_destroy(&attr)) failed=1;
  if(e||ae) { target.pid=0; failed=1; goto done; }
  target_pid=target.pid;
  if(failed) goto done;
  if(snprintf(pid,sizeof pid,"%d",target.pid)>=(int)sizeof pid) { failed=1; goto done; }
  char *ra[]={argv[7],"xctrace","record","--template","Time Profiler","--time-limit",argv[8],"--no-prompt","--output",trace,"--notify-tracing-started",name,"--attach",pid,NULL};
  if(posix_spawn_file_actions_init(&a)) { failed=1; goto done; }
  ae=actions(&a,streams,ends,2);
  if(!ae) e=posix_spawn(&recorder.pid,argv[7],&a,NULL,ra,environ); else e=ae;
  if(posix_spawn_file_actions_destroy(&a)) failed=1;
  if(e) { recorder.pid=0; failed=1; goto done; }
  recorder_pid=recorder.pid;
  if(failed) goto done;
  for(int i=0;i<4;i++) { close_checked(ends[i]); ends[i]=-1; }
  while(clock_ms()<start+total) {
    tick(&target,&recorder,streams,(uint64_t)limit);
    if(failed) break;
    char byte; ssize_t n=read(STDIN_FILENO,&byte,1);
    if(interrupted||n==0||n>0) { canceled=1; break; }
    if(n<0&&errno!=EAGAIN&&errno!=EINTR) { failed=1; break; }
    if(!ready) {
      if(!recorder.pid) { failed=1; break; }
      if(notify_check(token,&state)!=NOTIFY_STATUS_OK) { failed=1; break; }
      if(state) { ready=1; if(!signal_owned(&target,SIGCONT)) { failed=1; break; } resumed=1; }
      if(clock_ms()>start+readiness) { failed=1; break; }
    }
    if(!target.pid&&!recorder.pid) { int eof=1; for(int i=0;i<4;i++) if(streams[i].readfd>=0) eof=0; if(eof) break; }
    usleep(10000);
  }
  if(target.pid||recorder.pid) failed=1;
  for(int i=0;i<4;i++) if(streams[i].readfd>=0) failed=1;
done:
  /* Close inherited writers before cleanup so EOF is observable. Cleanup never
   * resumes the target: failed readiness cannot execute arbitrary main(). */
  for(int i=0;i<4;i++) { close_checked(ends[i]); ends[i]=-1; }
  cleanup(&target,&recorder,streams,(uint64_t)(limit>0?limit:1),cleanup_ms>0?cleanup_ms:1000);
  cleanup(&recorder,&target,streams,(uint64_t)(limit>0?limit:1),cleanup_ms>0?cleanup_ms:1000);
  tick(&target,&recorder,streams,(uint64_t)(limit>0?limit:1));
  for(int i=0;i<4;i++) { close_checked(streams[i].readfd); close_checked(streams[i].writefd); }
  if(token>=0&&notify_cancel(token)!=NOTIFY_STATUS_OK) failed=1;
  if(printf("{\"protocol\":1,\"targetPID\":%d,\"recorderPID\":%d,\"notification\":\"%s\",\"ready\":%s,\"resumed\":%s,\"targetReaped\":%s,\"recorderReaped\":%s,\"targetStatus\":%d,\"recorderStatus\":%d,\"forced\":%s,\"canceled\":%s,\"failed\":%s}\n",target_pid,recorder_pid,name,ready?"true":"false",resumed?"true":"false",target.reaped?"true":"false",recorder.reaped?"true":"false",target.status,recorder.status,target.forced||recorder.forced?"true":"false",canceled?"true":"false",failed?"true":"false")<0 || fflush(stdout)) return 2;
  return failed||target.pid||recorder.pid||canceled ? 1:0;
}
