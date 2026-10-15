// @ts-check

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'BuildWorld',
  tagline: 'BuildWorld is a self-hosted CI/CD server and Jenkins alternative for build automation, pipeline as code, and deployment workflows.',
  favicon: 'img/favicon.svg',
  url: 'https://neko233-com.github.io',
  baseUrl: '/buildworld/',
  organizationName: 'neko233-com',
  projectName: 'buildworld',
  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
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
      metadata: [
        {
          name: 'keywords',
          content: 'BuildWorld, buildworld, CI/CD, self-hosted CI/CD server, build automation, pipeline as code, Jenkins alternative, continuous integration, continuous delivery, 自托管 CI/CD 服务器, 持续集成, 持续交付, 构建自动化, Jenkins 替代方案',
        },
      ],
      navbar: {
        title: 'BuildWorld',
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
            href: 'https://github.com/neko233-com/buildworld',
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
                href: 'https://github.com/neko233-com/buildworld',
              },
              {
                label: 'Issues',
                href: 'https://github.com/neko233-com/buildworld/issues',
              },
            ],
          },
          {
            title: 'More',
            items: [
              {
                label: 'Changelog',
                href: 'https://github.com/neko233-com/buildworld/blob/main/CHANGELOG.md',
              },
              {
                label: 'License',
                href: 'https://github.com/neko233-com/buildworld/blob/main/LICENSE',
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
