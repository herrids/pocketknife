import { Routes, Route, Navigate } from "react-router-dom";
import { Login } from "./screens/Login";
import { Home } from "./screens/Home";
import { PlanReview } from "./screens/PlanReview";
import { AppView } from "./screens/AppView";
import { PrivateRoute } from "./components/PrivateRoute";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/home"
        element={
          <PrivateRoute>
            <Home />
          </PrivateRoute>
        }
      />
      <Route
        path="/plan/:sessionId"
        element={
          <PrivateRoute>
            <PlanReview />
          </PrivateRoute>
        }
      />
      <Route
        path="/app/:appId"
        element={
          <PrivateRoute>
            <AppView />
          </PrivateRoute>
        }
      />
      <Route path="*" element={<Navigate to="/home" replace />} />
    </Routes>
  );
}
