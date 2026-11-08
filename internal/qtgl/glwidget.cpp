// A QOpenGLWidget subclass whose GL callbacks and input events are
// forwarded to Go. miqt has no Qt 6 OpenGLWidgets binding, so this small
// shim provides one; Go wraps the result as a plain *qt6.QWidget.
#include <QOpenGLWidget>
#include <QOpenGLContext>
#include <QSurfaceFormat>
#include <QEvent>
#include <QInputMethodQueryEvent>
#include "glwidget.h"

extern "C" {
void gottiInitializeGL(uintptr_t h);
void gottiPaintGL(uintptr_t h);
void gottiResizeGL(uintptr_t h, int w, int hh);
int  gottiEvent(uintptr_t h, void* ev);
}

class GottiGL : public QOpenGLWidget {
public:
    uintptr_t handle;
    QRect imRect;
    void setIMRect(const QRect& r) {
        if (imRect != r) { imRect = r; updateMicroFocus(); }
    }
    GottiGL(uintptr_t h, QWidget* parent) : QOpenGLWidget(parent), handle(h) {
        setAttribute(Qt::WA_InputMethodEnabled, true);
        setAttribute(Qt::WA_KeyCompression, false);
        setFocusPolicy(Qt::StrongFocus);
        setMouseTracking(true);
        setUpdateBehavior(QOpenGLWidget::NoPartialUpdate);
    }
protected:
    void initializeGL() override { gottiInitializeGL(handle); }
    void paintGL() override { gottiPaintGL(handle); }
    void resizeGL(int w, int h) override { gottiResizeGL(handle, w, h); }
    bool focusNextPrevChild(bool) override { return false; } // Tab belongs to the terminal
    bool event(QEvent* e) override {
        switch (e->type()) {
        case QEvent::KeyPress: case QEvent::KeyRelease: case QEvent::ShortcutOverride:
        case QEvent::MouseButtonPress: case QEvent::MouseButtonRelease:
        case QEvent::MouseButtonDblClick: case QEvent::MouseMove: case QEvent::Wheel:
        case QEvent::FocusIn: case QEvent::FocusOut: case QEvent::InputMethod:
        case QEvent::Enter: case QEvent::Leave:
            if (gottiEvent(handle, e)) { e->accept(); return true; }
            break;
        default:
            break;
        }
        return QOpenGLWidget::event(e);
    }
    QVariant inputMethodQuery(Qt::InputMethodQuery q) const override {
        switch (q) {
        case Qt::ImEnabled: return true;
        case Qt::ImCursorRectangle: return imRect;
        case Qt::ImAnchorRectangle: return imRect;
        case Qt::ImCursorPosition: return 0;
        case Qt::ImSurroundingText: return QString();
        case Qt::ImHints: return int(Qt::ImhNoPredictiveText | Qt::ImhNoAutoUppercase);
        default: return QOpenGLWidget::inputMethodQuery(q);
        }
    }
};

extern "C" {
void* gotti_glwidget_new(uintptr_t handle, void* parent) {
    return static_cast<QWidget*>(new GottiGL(handle, static_cast<QWidget*>(parent)));
}
void gotti_glwidget_set_im_rect(void* w, int x, int y, int width, int height) {
    auto* g = static_cast<GottiGL*>(static_cast<QWidget*>(w));
    g->setIMRect(QRect(x, y, width, height));
}
unsigned int gotti_glwidget_default_fbo(void* w) {
    return static_cast<GottiGL*>(static_cast<QWidget*>(w))->defaultFramebufferObject();
}
void gotti_glwidget_make_current(void* w) { static_cast<GottiGL*>(static_cast<QWidget*>(w))->makeCurrent(); }
void gotti_glwidget_done_current(void* w) { static_cast<GottiGL*>(static_cast<QWidget*>(w))->doneCurrent(); }
void* gotti_get_proc_address(const char* name) {
    QOpenGLContext* ctx = QOpenGLContext::currentContext();
    if (!ctx) return nullptr;
    return reinterpret_cast<void*>(ctx->getProcAddress(name));
}
}
