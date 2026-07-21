// @ts-check

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'buildworld',
  tagline: 'A modern CI/CD server - Jenkins alternative',
  favicon: 'img/favicon.svg',
  url: 'https://neko233-com.github.io',
  baseUrl: '/buildworld233/',
  organizationName: 'neko233-com',
  projectName: 'buildworld233',
  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'zh-Hans'],
    localeConfigs: {
      en: {
        htmlLang: 'en-US',
      },
      'zh-Hans': {
        htmlLang: 'zh-Hans',
      },
    },
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: './sidebars.js',
          routeBasePath: '/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      navbar: {
        title: 'buildworld',
        logo: {
          alt: 'buildworld Logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            position: 'left',
            label: 'Documentation',
          },
          {
            type: 'localeDropdown',
            position: 'right',
          },
          {
            href: 'https://github.com/neko233-com/buildworld233',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Documentation',
            items: [
              {
                label: 'Getting Started',
                to: '/',
              },
              {
                label: 'Installation',
                to: '/installation',
              },
              {
                label: 'Configuration',
                to: '/configuration',
              },
            ],
          },
          {
            title: 'Community',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/neko233-com/buildworld233',
              },
              {
                label: 'Issues',
                href: 'https://github.com/neko233-com/buildworld233/issues',
              },
            ],
          },
          {
            title: 'More',
            items: [
              {
                label: 'Changelog',
                href: 'https://github.com/neko233-com/buildworld233/blob/main/CHANGELOG.md',
              },
              {
                label: 'License',
                href: 'https://github.com/neko233-com/buildworld233/blob/main/LICENSE',
              },
            ],
          },
        ],
        copyright: `Copyright ${new Date().getFullYear()} buildworld. Built with Docusaurus.`,
      },
      prism: {
        additionalLanguages: ['go', 'typescript', 'bash', 'yaml', 'json'],
      },
    }),
};

export default config;
