#include "backend.h"
#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>

int main(int argc, char *argv[]) {
    QQuickStyle::setStyle("Basic");
    QGuiApplication app(argc, argv);
    app.setApplicationName("UtauTTS");
    app.setOrganizationName("UtauTTS");

    Backend backend;
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("backend", &backend);
    engine.loadFromModule("UtauTTS", "Main");
    if (engine.rootObjects().isEmpty()) {
        return -1;
    }

    backend.initialize();
    return app.exec();
}
