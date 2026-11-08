#pragma once
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
void* gotti_glwidget_new(uintptr_t handle, void* parent);
void  gotti_glwidget_set_im_rect(void* w, int x, int y, int width, int height);
unsigned int gotti_glwidget_default_fbo(void* w);
void  gotti_glwidget_make_current(void* w);
void  gotti_glwidget_done_current(void* w);
void* gotti_get_proc_address(const char* name);
#ifdef __cplusplus
}
#endif
