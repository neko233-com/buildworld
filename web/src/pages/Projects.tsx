export default function Projects() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Projects</h1>
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Name</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Status</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Last Build</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-b">
              <td className="px-6 py-4">my-app</td>
              <td className="px-6 py-4"><span className="text-green-600">Active</span></td>
              <td className="px-6 py-4">2 minutes ago</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  )
}
