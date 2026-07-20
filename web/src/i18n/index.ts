import { useCallback, useSyncExternalStore } from 'react';
import en from './en.json';
import zhCN from './zh-CN.json';

export type Locale = 'en' | 'zh-CN' | 'ja-JP' | 'ko-KR' | 'ru-RU' | 'hi-IN';
export const localeLabels: Record<Locale, string> = { en: 'English', 'zh-CN': '中文', 'ja-JP': '日本語', 'ko-KR': '한국어', 'ru-RU': 'Русский', 'hi-IN': 'हिन्दी' };
type RegionalPluginCopy = Partial<typeof en.pluginRegistry>;
type RegionalAgentsCopy = Partial<typeof en.agents>;

const regionalShell: Record<Exclude<Locale, 'en' | 'zh-CN'>, Partial<typeof en>> = {
  'ja-JP': { app: { ...en.app, tagline: 'モダン CI/CD サーバー' }, nav: { ...en.nav, dashboard: 'ダッシュボード', projects: 'プロジェクト', pipeline: 'パイプライン', builds: 'ビルド', agents: 'エージェント', settings: '設定' }, shell: { ...en.shell, workspace: 'ワークスペース', execution: '実行', administration: '管理', searchPlaceholder: 'プロジェクト、ビルド、コマンドを検索', logout: 'ログアウト', globalSearch: 'グローバル検索', projects: 'プロジェクト', builds: 'ビルド', commands: 'コマンド', noResults: '一致するプロジェクト、ビルド、コマンドはありません。', projectsDetail: 'プロジェクトのパイプラインを管理', buildQueueDetail: '優先度と待機中のビルド', workersDetail: '登録済み実行ノード', notificationsDetail: '共有通知チャネル', pluginsDetail: 'GitHub から Go バイナリプラグインをインストール' } },
  'ko-KR': { app: { ...en.app, tagline: '현대적인 CI/CD 서버' }, nav: { ...en.nav, dashboard: '대시보드', projects: '프로젝트', pipeline: '파이프라인', builds: '빌드', agents: '에이전트', settings: '설정' }, shell: { ...en.shell, workspace: '작업 공간', execution: '실행', administration: '관리', searchPlaceholder: '프로젝트, 빌드, 명령 검색', logout: '로그아웃', globalSearch: '전역 검색', projects: '프로젝트', builds: '빌드', commands: '명령', noResults: '일치하는 프로젝트, 빌드 또는 명령이 없습니다.', projectsDetail: '프로젝트 파이프라인 관리', buildQueueDetail: '우선순위 및 대기 중인 빌드', workersDetail: '등록된 실행 노드', notificationsDetail: '공유 알림 채널', pluginsDetail: 'GitHub에서 Go 바이너리 플러그인 설치' } },
  'ru-RU': { app: { ...en.app, tagline: 'Современный CI/CD сервер' }, nav: { ...en.nav, dashboard: 'Панель', projects: 'Проекты', pipeline: 'Пайплайны', builds: 'Сборки', agents: 'Агенты', settings: 'Настройки' }, shell: { ...en.shell, workspace: 'Рабочая область', execution: 'Выполнение', administration: 'Администрирование', searchPlaceholder: 'Поиск проектов, сборок и команд', logout: 'Выйти', globalSearch: 'Глобальный поиск', projects: 'Проекты', builds: 'Сборки', commands: 'Команды', noResults: 'Нет подходящих проектов, сборок или команд.', projectsDetail: 'Управление пайплайнами проектов', buildQueueDetail: 'Приоритеты и ожидающие сборки', workersDetail: 'Зарегистрированные узлы выполнения', notificationsDetail: 'Общие каналы уведомлений', pluginsDetail: 'Установка Go-бинарных плагинов из GitHub' } },
  'hi-IN': { app: { ...en.app, tagline: 'आधुनिक CI/CD सर्वर' }, nav: { ...en.nav, dashboard: 'डैशबोर्ड', projects: 'प्रोजेक्ट', pipeline: 'पाइपलाइन', builds: 'बिल्ड', agents: 'एजेंट', settings: 'सेटिंग्स' }, shell: { ...en.shell, workspace: 'कार्यस्थान', execution: 'निष्पादन', administration: 'प्रशासन', searchPlaceholder: 'प्रोजेक्ट, बिल्ड और कमांड खोजें', logout: 'लॉग आउट', globalSearch: 'वैश्विक खोज', projects: 'प्रोजेक्ट', builds: 'बिल्ड', commands: 'कमांड', noResults: 'कोई मेल खाता प्रोजेक्ट, बिल्ड या कमांड नहीं मिला।', projectsDetail: 'प्रोजेक्ट पाइपलाइन ब्राउज़ और प्रबंधित करें', buildQueueDetail: 'प्राथमिकताएं और प्रतीक्षारत बिल्ड', workersDetail: 'पंजीकृत निष्पादन नोड', notificationsDetail: 'साझा सूचना चैनल', pluginsDetail: 'GitHub से Go बाइनरी प्लगइन इंस्टॉल करें' } },
};

const regionalPluginCopy: Record<Exclude<Locale, 'en' | 'zh-CN'>, RegionalPluginCopy> = {
  'ja-JP': { title: 'プラグインソース', description: '公開 GitHub リポジトリから、独立してビルドされた Go プラグインを直接インストールします。', installed: '件をインストール済み', repositoryUrl: 'GitHub リポジトリ URL', install: 'プラグインをインストール', building: 'プラグインをビルド中', installHelp: 'plugin-buildworld.json が必要です。ソースビルドは SHA-256 で検証され、同名・同バージョンは再利用されます。', all: 'すべて', enabled: '有効', disabled: '無効', version: 'バージョン', capabilities: '機能', integrity: '整合性', status: '状態', noSteps: '実行可能なステップはありません', legacy: '互換モード', noDescription: 'プラグインの説明はありません', noMatches: 'このフィルターに一致するプラグインはありません。', removeTitle: 'プラグインを削除', removeDescription: 'インストール済みファイルと登録済み機能が削除されます。', remove: '削除', removing: '削除中...', installedNotice: 'インストール済み', reloaded: 'を再読み込みしました。', removed: 'を削除しました。', installFailed: 'この Go プラグインをインストールできません。', toggleFailed: 'プラグインの状態を変更できません。', reloadFailed: 'プラグインを再読み込みできません。', removeFailed: 'プラグインを削除できません。' },
  'ko-KR': { title: '플러그인 소스', description: '공개 GitHub 저장소에서 독립적으로 빌드된 Go 플러그인을 바로 설치합니다.', installed: '개 설치됨', repositoryUrl: 'GitHub 저장소 URL', install: '플러그인 설치', building: '플러그인 빌드 중', installHelp: 'plugin-buildworld.json이 필요합니다. 소스 빌드는 SHA-256으로 검증되며 같은 이름과 버전은 재사용됩니다.', all: '전체', enabled: '사용', disabled: '사용 안 함', version: '버전', capabilities: '기능', integrity: '무결성', status: '상태', noSteps: '실행 가능한 단계 없음', legacy: '호환 모드', noDescription: '플러그인 설명 없음', noMatches: '이 필터와 일치하는 플러그인이 없습니다.', removeTitle: '플러그인 제거', removeDescription: '설치된 플러그인 파일과 등록된 기능을 제거합니다.', remove: '제거', removing: '제거 중...', installedNotice: '설치됨', reloaded: '을(를) 다시 로드했습니다.', removed: '을(를) 제거했습니다.', installFailed: '이 Go 플러그인을 설치할 수 없습니다.', toggleFailed: '플러그인 상태를 변경할 수 없습니다.', reloadFailed: '플러그인을 다시 로드할 수 없습니다.', removeFailed: '플러그인을 제거할 수 없습니다.' },
  'ru-RU': { title: 'Источники плагинов', description: 'Устанавливайте независимо собранные Go-плагины напрямую из открытого репозитория GitHub.', installed: 'установлено', repositoryUrl: 'URL репозитория GitHub', install: 'Установить плагин', building: 'Сборка плагина', installHelp: 'Требуется plugin-buildworld.json. Сборки исходников проверяются SHA-256, совпадающие имя и версия используются повторно.', all: 'Все', enabled: 'Включено', disabled: 'Выключено', version: 'Версия', capabilities: 'Возможности', integrity: 'Целостность', status: 'Статус', noSteps: 'Нет исполняемых шагов', legacy: 'Совместимость', noDescription: 'Описание плагина отсутствует', noMatches: 'Нет плагинов для этого фильтра.', removeTitle: 'Удалить плагин', removeDescription: 'Будут удалены файлы установленного плагина и зарегистрированные возможности.', remove: 'Удалить', removing: 'Удаление...', installedNotice: 'Установлен', reloaded: 'перезагружен.', removed: 'удален.', installFailed: 'Не удалось установить этот Go-плагин.', toggleFailed: 'Не удалось изменить состояние плагина.', reloadFailed: 'Не удалось перезагрузить плагин.', removeFailed: 'Не удалось удалить плагин.' },
  'hi-IN': { title: 'प्लगइन स्रोत', description: 'सार्वजनिक GitHub रिपॉजिटरी से स्वतंत्र रूप से बने Go प्लगइन सीधे इंस्टॉल करें।', installed: 'इंस्टॉल किए गए', repositoryUrl: 'GitHub रिपॉजिटरी URL', install: 'प्लगइन इंस्टॉल करें', building: 'प्लगइन बनाया जा रहा है', installHelp: 'plugin-buildworld.json आवश्यक है। सोर्स बिल्ड SHA-256 से सत्यापित होते हैं और समान नाम/संस्करण दोबारा उपयोग होते हैं।', all: 'सभी', enabled: 'सक्षम', disabled: 'अक्षम', version: 'संस्करण', capabilities: 'क्षमताएं', integrity: 'अखंडता', status: 'स्थिति', noSteps: 'कोई निष्पादन योग्य चरण नहीं', legacy: 'संगतता मोड', noDescription: 'कोई प्लगइन विवरण नहीं', noMatches: 'इस फ़िल्टर से कोई प्लगइन मेल नहीं खाता।', removeTitle: 'प्लगइन हटाएं', removeDescription: 'इंस्टॉल की गई प्लगइन फाइलें और पंजीकृत क्षमताएं हटा दी जाएंगी।', remove: 'प्लगइन हटाएं', removing: 'हटाया जा रहा है...', installedNotice: 'इंस्टॉल किया गया', reloaded: 'पुनः लोड किया गया।', removed: 'हटा दिया गया।', installFailed: 'यह Go प्लगइन इंस्टॉल नहीं किया जा सका।', toggleFailed: 'प्लगइन स्थिति नहीं बदली जा सकी।', reloadFailed: 'प्लगइन पुनः लोड नहीं किया जा सका।', removeFailed: 'प्लगइन हटाया नहीं जा सका।' },
};

const regionalAgentsCopy: Record<Exclude<Locale, 'en' | 'zh-CN'>, RegionalAgentsCopy> = {
  'ja-JP': { title: 'エージェント', register: 'エージェントを登録', online: 'オンライン', offline: 'オフライン', total: 'エージェント総数', copy: 'コピー', dismiss: '閉じる', addressOptional: 'アドレス（任意）', labelsHint: 'ラベル（カンマ区切り）', maxConcurrent: '最大同時ビルド数', defaultPool: '既定' },
  'ko-KR': { title: '에이전트', register: '에이전트 등록', online: '온라인', offline: '오프라인', total: '전체 에이전트', copy: '복사', dismiss: '닫기', addressOptional: '주소 (선택 사항)', labelsHint: '레이블 (쉼표로 구분)', maxConcurrent: '최대 동시 빌드', defaultPool: '기본값' },
  'ru-RU': { title: 'Агенты', register: 'Зарегистрировать агента', online: 'Онлайн', offline: 'Офлайн', total: 'Всего агентов', copy: 'Копировать', dismiss: 'Закрыть', addressOptional: 'Адрес (необязательно)', labelsHint: 'Метки (через запятую)', maxConcurrent: 'Максимум параллельных сборок', defaultPool: 'По умолчанию' },
  'hi-IN': { title: 'एजेंट', register: 'एजेंट पंजीकृत करें', online: 'ऑनलाइन', offline: 'ऑफलाइन', total: 'कुल एजेंट', copy: 'कॉपी करें', dismiss: 'बंद करें', addressOptional: 'पता (वैकल्पिक)', labelsHint: 'लेबल (कॉमा से अलग)', maxConcurrent: 'अधिकतम समवर्ती बिल्ड', defaultPool: 'डिफ़ॉल्ट' },
};

const translations: Record<Locale, typeof en> = {
  'en': en,
  'zh-CN': zhCN,
  'ja-JP': { ...en, ...regionalShell['ja-JP'], nav: { ...en.nav, ...regionalShell['ja-JP'].nav }, app: { ...en.app, ...regionalShell['ja-JP'].app }, agents: { ...en.agents, ...regionalAgentsCopy['ja-JP'] }, pluginRegistry: { ...en.pluginRegistry, ...regionalPluginCopy['ja-JP'] } },
  'ko-KR': { ...en, ...regionalShell['ko-KR'], nav: { ...en.nav, ...regionalShell['ko-KR'].nav }, app: { ...en.app, ...regionalShell['ko-KR'].app }, agents: { ...en.agents, ...regionalAgentsCopy['ko-KR'] }, pluginRegistry: { ...en.pluginRegistry, ...regionalPluginCopy['ko-KR'] } },
  'ru-RU': { ...en, ...regionalShell['ru-RU'], nav: { ...en.nav, ...regionalShell['ru-RU'].nav }, app: { ...en.app, ...regionalShell['ru-RU'].app }, agents: { ...en.agents, ...regionalAgentsCopy['ru-RU'] }, pluginRegistry: { ...en.pluginRegistry, ...regionalPluginCopy['ru-RU'] } },
  'hi-IN': { ...en, ...regionalShell['hi-IN'], nav: { ...en.nav, ...regionalShell['hi-IN'].nav }, app: { ...en.app, ...regionalShell['hi-IN'].app }, agents: { ...en.agents, ...regionalAgentsCopy['hi-IN'] }, pluginRegistry: { ...en.pluginRegistry, ...regionalPluginCopy['hi-IN'] } },
};

function detectBrowserLocale(): Locale {
  const langs = navigator.languages || [navigator.language];
  for (const lang of langs) {
    if (lang.startsWith('zh')) return 'zh-CN';
    if (lang.startsWith('en')) return 'en';
    if (lang.startsWith('ja')) return 'ja-JP';
    if (lang.startsWith('ko')) return 'ko-KR';
    if (lang.startsWith('ru')) return 'ru-RU';
    if (lang.startsWith('hi')) return 'hi-IN';
  }
  // 无法识别时默认中文，匹配中文用户环境
  return 'zh-CN';
}

function savedLocale(): Locale {
  const saved = localStorage.getItem('locale') as Locale | null;
  return saved && saved in translations ? saved : detectBrowserLocale();
}

let activeLocale: Locale = savedLocale();
const localeListeners = new Set<() => void>();

function subscribeLocale(listener: () => void) {
  localeListeners.add(listener);
  return () => localeListeners.delete(listener);
}

function getActiveLocale(): Locale {
  return activeLocale;
}

function setActiveLocale(locale: Locale) {
  if (activeLocale === locale) return;
  activeLocale = locale;
  localStorage.setItem('locale', locale);
  localeListeners.forEach(listener => listener());
}

export function useI18n() {
  const locale = useSyncExternalStore(subscribeLocale, getActiveLocale, getActiveLocale);

  const t = useCallback((key: string): string => {
    const keys = key.split('.');
    let value: any = translations[locale];
    
    for (const k of keys) {
      value = value?.[k];
    }
    
    return value || key;
  }, [locale]);

  const changeLocale = useCallback((newLocale: Locale) => {
    setActiveLocale(newLocale);
  }, []);

  return {
    locale,
    t,
    changeLocale,
    locales: Object.keys(translations) as Locale[],
  };
}
