import React from 'react';
import ComponentCreator from '@docusaurus/ComponentCreator';

export default [
  {
    path: '/zh-Hans/',
    component: ComponentCreator('/zh-Hans/', 'c4a'),
    routes: [
      {
        path: '/zh-Hans/',
        component: ComponentCreator('/zh-Hans/', '1f8'),
        routes: [
          {
            path: '/zh-Hans/',
            component: ComponentCreator('/zh-Hans/', '389'),
            routes: [
              {
                path: '/zh-Hans/agents',
                component: ComponentCreator('/zh-Hans/agents', '0d3'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/configuration',
                component: ComponentCreator('/zh-Hans/configuration', 'bc0'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/installation',
                component: ComponentCreator('/zh-Hans/installation', '1d7'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/notifications',
                component: ComponentCreator('/zh-Hans/notifications', 'ac4'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/pipelines',
                component: ComponentCreator('/zh-Hans/pipelines', 'b49'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/plugins',
                component: ComponentCreator('/zh-Hans/plugins', '15b'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/templates',
                component: ComponentCreator('/zh-Hans/templates', 'e55'),
                exact: true,
                sidebar: "docsSidebar"
              },
              {
                path: '/zh-Hans/',
                component: ComponentCreator('/zh-Hans/', 'fc5'),
                exact: true,
                sidebar: "docsSidebar"
              }
            ]
          }
        ]
      }
    ]
  },
  {
    path: '*',
    component: ComponentCreator('*'),
  },
];
