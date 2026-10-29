import { defineConfig, type HeadConfig } from 'vitepress'

// Served from https://ercross.github.io/reconD/ by .github/workflows/docs.yml.
const siteUrl = 'https://ercross.github.io/reconD/'
const siteDescription =
  'reconD is a lightweight GitOps-style continuous deployment agent for single-host Docker. It deploys the image your CI released, health-checks it, and rolls back automatically on failure.'
const socialImage = `${siteUrl}og-image.png`

// Describes reconD to search engines as a downloadable application.
const softwareApplication = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: 'reconD',
  description: siteDescription,
  url: siteUrl,
  applicationCategory: 'DeveloperApplication',
  operatingSystem: 'Linux',
  license: 'https://opensource.org/licenses/MIT',
  downloadUrl: 'https://github.com/ercross/reconD/releases',
  offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
}

export default defineConfig({
  title: 'reconD',
  titleTemplate: ':title | reconD',
  description: siteDescription,
  base: '/reconD/',
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: siteUrl },
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/reconD/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#3b82f6' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'reconD' }],
    ['meta', { property: 'og:image', content: socialImage }],
    ['meta', { property: 'og:image:width', content: '1200' }],
    ['meta', { property: 'og:image:height', content: '630' }],
    ['meta', { property: 'og:image:alt', content: 'reconD: continuous deployment for single-host Docker' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:image', content: socialImage }],
  ],

  // Adds the per-page tags that depend on the page's URL, title, and description.
  transformPageData(pageData) {
    const path = pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    const url = siteUrl + path
    const title = pageData.frontmatter.layout === 'home'
      ? pageData.frontmatter.title
      : `${pageData.frontmatter.title ?? pageData.title} | reconD`
    const description = pageData.frontmatter.description ?? siteDescription

    const head: HeadConfig[] = [
      ['link', { rel: 'canonical', href: url }],
      ['meta', { property: 'og:url', content: url }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
    ]
    if (pageData.frontmatter.layout === 'home') {
      head.push(['script', { type: 'application/ld+json' }, JSON.stringify(softwareApplication)])
    }
    pageData.frontmatter.head = [...(pageData.frontmatter.head ?? []), ...head]
  },

  markdown: {
    theme: { light: 'github-light', dark: 'github-dark' },
  },

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Guide', link: '/guide/what-is-recond', activeMatch: '/guide/' },
      { text: 'Install', link: '/install/', activeMatch: '/install/' },
      { text: 'Configuration', link: '/config/', activeMatch: '/config/' },
      { text: 'Operations', link: '/operations/state', activeMatch: '/operations/' },
      {
        text: 'More',
        items: [
          { text: 'FAQ', link: '/guide/faq' },
          { text: 'CLI reference', link: '/reference/cli' },
          { text: 'Deploy command environment', link: '/reference/deploy-command-env' },
          { text: 'Contributing', link: '/contributing/development' },
          { text: 'Changelog', link: '/changelog' },
          { text: 'Releases', link: 'https://github.com/ercross/reconD/releases' },
        ],
      },
    ],

    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'What is reconD?', link: '/guide/what-is-recond' },
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'How it works', link: '/guide/how-it-works' },
          { text: 'Deployment metadata', link: '/guide/deployment-metadata' },
          { text: 'FAQ', link: '/guide/faq' },
        ],
      },
      {
        text: 'Installation',
        items: [
          { text: 'Linux packages', link: '/install/' },
          { text: 'Running the service', link: '/install/service' },
          { text: 'Upgrade and uninstall', link: '/install/upgrade' },
          { text: 'Build from source', link: '/install/from-source' },
        ],
      },
      {
        text: 'Configuration',
        items: [
          { text: 'Overview', link: '/config/' },
          { text: 'Field reference', link: '/config/reference' },
          { text: 'Deploy command', link: '/config/deploy-command' },
          { text: 'Deployment strategies', link: '/config/strategies' },
          { text: 'Health checks', link: '/config/health-checks' },
          { text: 'Retries', link: '/config/retries' },
          { text: 'Notifications', link: '/config/notifications' },
        ],
      },
      {
        text: 'Operations',
        items: [
          { text: 'Deployment state', link: '/operations/state' },
          { text: 'Rollback and terminal failures', link: '/operations/rollback' },
          { text: 'Logging', link: '/operations/logging' },
          { text: 'Security model', link: '/operations/security' },
          { text: 'Troubleshooting', link: '/operations/troubleshooting' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI', link: '/reference/cli' },
          { text: 'Deploy command environment', link: '/reference/deploy-command-env' },
          { text: 'Changelog', link: '/changelog' },
        ],
      },
      {
        text: 'Contributing',
        items: [
          { text: 'Development', link: '/contributing/development' },
          { text: 'Packaging internals', link: '/contributing/packaging' },
        ],
      },
    ],

    socialLinks: [{ icon: 'github', link: 'https://github.com/ercross/reconD' }],

    editLink: {
      pattern: 'https://github.com/ercross/reconD/edit/master/docs/:path',
      text: 'Edit this page on GitHub',
    },

    search: { provider: 'local' },

    outline: { level: [2, 3] },

    footer: {
      message: 'Released under the MIT License.',
    },
  },
})
