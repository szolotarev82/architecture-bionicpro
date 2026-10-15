import React, { useState } from 'react';
import { useKeycloak } from '@react-keycloak/web';

interface DeviceReport {
  user_login: string;
  device_id: string;
  hour_bucket: string;
  avg_emg: number;
  avg_confidence: number;
  avg_battery_lvl: number;
  avg_battery_temp: number;
  grasp_count: number;
  rotate_count: number;
  total_events: number;
  error_count: number;
}

const ReportPage: React.FC = () => {
  const { keycloak, initialized } = useKeycloak();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reports, setReports] = useState<DeviceReport[]>([]);

  const downloadReport = async () => {
    if (!keycloak) {
      setError('Keycloak not initialized');
      return;
    }

    try {
      setLoading(true);
      setError(null);

      const response = await fetch(`${process.env.REACT_APP_API_URL}/reports`, {
        method: 'GET',
        headers: {
          'Authorization': `Bearer ${keycloak.token}`
        }
      });

      const responseText = await response.text();

      if (!response.ok) {
        let errorMessage = 'Unknown error';
        try {
          const jsonErr = JSON.parse(responseText);
          errorMessage = jsonErr.message || responseText;
        } catch (e) {
          errorMessage = responseText; 
        }
        throw new Error(errorMessage);
      }

      const data: DeviceReport[] = JSON.parse(responseText);
      setReports(data);

      if (data.length > 0) {
        alert(`Успешно загружено записей: ${data.length} для пользователя ${data[0].user_login}`);
      } else {
        alert('Записи отчетов для данного пользователя не найдены');
      }

    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
    } finally {
      setLoading(false);
    }
  };

  if (!initialized) {
    return <div className="text-center p-8">Loading Keycloak...</div>;
  }

  if (!keycloak.authenticated) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen bg-gray-100">
        <div className="p-8 bg-white rounded-lg shadow-md">
          <h1 className="text-2xl font-bold mb-6">BionicCore Reports</h1>
          <p className="mb-4">Please log in to access your prosthesis report.</p>
          <button
            onClick={() => keycloak.login()}
            className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
          >
            Login with Keycloak
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-gray-100 p-4">
      <div className="w-full max-w-2xl p-8 bg-white rounded-lg shadow-md">
        <h1 className="text-2xl font-bold mb-6">Usage Reports</h1>
        
        <button
          onClick={downloadReport}
          disabled={loading}
          className={`w-full py-2 bg-blue-500 text-white font-semibold rounded hover:bg-blue-600 transition ${
            loading ? 'opacity-50 cursor-not-allowed' : ''
          }`}
        >
          {loading ? 'Generating Report...' : 'Download My Report'}
        </button>

        {error && (
          <div className="mt-4 p-3 bg-red-50 text-red-600 border border-red-200 rounded">
            <strong>Error:</strong> {error}
          </div>
        )}

        {reports.length > 0 && (
          <div className="mt-6 overflow-x-auto">
            <table className="min-w-full bg-white border border-gray-200 text-sm">
              <thead>
                <tr className="bg-gray-50 border-b border-gray-200">
                  <th className="px-4 py-2 text-left text-gray-600">Device ID</th>
                  <th className="px-4 py-2 text-left text-gray-600">Hour</th>
                  <th className="px-4 py-2 text-center text-gray-600">Grasps</th>
                  <th className="px-4 py-2 text-center text-gray-600">Rotates</th>
                  <th className="px-4 py-2 text-center text-gray-600">Errors</th>
                </tr>
              </thead>
              <tbody>
                {reports.map((report, idx) => (
                  <tr key={idx} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-2 font-mono">{report.device_id}</td>
                    <td className="px-4 py-2">{new Date(report.hour_bucket).toLocaleString()}</td>
                    <td className="px-4 py-2 text-center font-semibold text-green-600">{report.grasp_count}</td>
                    <td className="px-4 py-2 text-center font-semibold text-blue-600">{report.rotate_count}</td>
                    <td className="px-4 py-2 text-center font-semibold text-red-600">{report.error_count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default ReportPage;
