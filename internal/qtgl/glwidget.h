#pragma once
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
void* termite_glwidget_new(uintptr_t handle, void* parent);
void  termite_glwidget_set_im_rect(void* w, int x, int y, int width, int height);
void  termite_glwidget_make_current(void* w);
void  termite_glwidget_done_current(void* w);
void* termite_get_proc_address(const char* name);
#ifdef __cplusplus
}
#endif
