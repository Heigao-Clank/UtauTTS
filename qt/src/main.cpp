#include "backend.h"
#include <QFile>
#include <QGuiApplication>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQuickStyle>
#include <QUrl>
#include <QVariantList>

namespace {
QString readTextResource(const QString &path) {
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
        return {};
    }
    return QString::fromUtf8(file.readAll()).trimmed();
}

QVariantList legalDocuments() {
    QVariantList documents;
    documents.append(QVariantMap{{"name", "UtauTTS"},
                                 {"text", readTextResource(":/legal/LICENSE")}});

    const QString notices = readTextResource(":/legal/THIRD_PARTY_NOTICES.txt");
    const QStringList lines = notices.split('\n');
    int sectionStart = 0;
    for (int index = 1; index < lines.size(); ++index) {
        const QString underline = lines.at(index).trimmed();
        if (underline.size() < 3 || underline.count('=') != underline.size()) {
            continue;
        }
        const int headingIndex = index - 1;
        if (headingIndex > sectionStart) {
            const QString text = lines.mid(sectionStart, headingIndex - sectionStart).join('\n').trimmed();
            if (!text.isEmpty()) {
                documents.append(QVariantMap{{"name", lines.at(sectionStart).trimmed()},
                                             {"text", text}});
            }
        }
        sectionStart = headingIndex;
    }
    const QString finalSection = lines.mid(sectionStart).join('\n').trimmed();
    if (!finalSection.isEmpty()) {
        documents.append(QVariantMap{{"name", lines.at(sectionStart).trimmed()},
                                     {"text", finalSection}});
    }
    return documents;
}
} // namespace

int main(int argc, char *argv[]) {
    QQuickStyle::setStyle("Fusion");
    QGuiApplication app(argc, argv);
    app.setApplicationName(UTAUTTS_APP_NAME);
    app.setApplicationDisplayName(UTAUTTS_APP_NAME);
    app.setApplicationVersion(UTAUTTS_VERSION);
    app.setOrganizationName(UTAUTTS_APP_ORGANIZATION);

    QIcon appIcon;
    appIcon.addFile(QStringLiteral(":/icons/icon16.png"));
    appIcon.addFile(QStringLiteral(":/icons/icon32.png"));
    appIcon.addFile(QStringLiteral(":/icons/icon64.png"));
    appIcon.addFile(QStringLiteral(":/icons/icon128.png"));
    appIcon.addFile(QStringLiteral(":/icons/icon512.png"));
    app.setWindowIcon(appIcon);

    Backend backend;
    QQmlApplicationEngine engine;
    engine.setInitialProperties({
        {"injectedBackend", QVariant::fromValue(static_cast<QObject *>(&backend))},
        {"injectedLegalDocuments", legalDocuments()},
        {"injectedAppName", QStringLiteral(UTAUTTS_APP_NAME)},
        {"injectedRepositoryUrl", QUrl(QStringLiteral(UTAUTTS_APP_REPOSITORY))},
    });
    engine.loadFromModule("UtauTTS", "Main");
    if (engine.rootObjects().isEmpty()) {
        return -1;
    }

    backend.initialize();
    return app.exec();
}
